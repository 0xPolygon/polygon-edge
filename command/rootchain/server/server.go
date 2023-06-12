package server

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	dockerclient "github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-connections/nat"
	"github.com/hashicorp/go-uuid"
	"github.com/spf13/cobra"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/keystore"
	"github.com/umbracle/ethgo/wallet"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/rootchain/helper"
	"github.com/0xPolygon/polygon-edge/helper/common"
)

var (
	//go:embed genesis.json.tmpl
	genesisTmpl string
)

const (
	gethConsoleImage = "ghcr.io/0xpolygon/go-ethereum-console:latest"
	gethImage        = "ethereum/client-go:v1.9.25"

	defaultHostIP   = "127.0.0.1"
	defaultHostPort = "8545"
)

var (
	params            serverParams
	dockerClient      *dockerclient.Client
	dockerContainerID string
)

// GetCommand returns the rootchain server command
func GetCommand() *cobra.Command {
	rootchainServerCmd := &cobra.Command{
		Use:     "server",
		Short:   "Start the rootchain command",
		PreRunE: runPreRun,
		Run:     runCommand,
	}

	setFlags(rootchainServerCmd)

	return rootchainServerCmd
}

func setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(
		&params.dataDir,
		dataDirFlag,
		"test-rootchain",
		"target directory for the chain",
	)

	cmd.Flags().BoolVar(
		&params.noConsole,
		noConsole,
		false,
		"use the official geth image instead of the console fork",
	)

	cmd.Flags().StringArrayVar(
		&params.premine,
		premineFlag,
		[]string{},
		"premine accounts at genesis",
	)
}

func runPreRun(_ *cobra.Command, _ []string) error {
	return nil
}

func runCommand(cmd *cobra.Command, _ []string) {
	ctx := cmd.Context()

	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	closeCh := make(chan struct{})

	// Check if the client is already running
	if cid, err := helper.GetRootchainID(); !errors.Is(err, helper.ErrRootchainNotFound) {
		if err != nil {
			outputter.SetError(err)
		} else if cid != "" {
			outputter.SetError(fmt.Errorf("rootchain already running: %s", cid))
		}

		return
	}

	// Start the client
	if err := runRootchain(ctx, outputter, closeCh); err != nil {
		outputter.SetError(fmt.Errorf("failed to run rootchain: %w", err))

		return
	}

	// Ping geth server to make sure everything is up and running
	if err := PingServer(closeCh); err != nil {
		close(closeCh)

		if ip, err := helper.ReadRootchainIP(); err != nil {
			outputter.SetError(fmt.Errorf("failed to ping rootchain server: %w", err))
		} else {
			outputter.SetError(fmt.Errorf("failed to ping rootchain server at address %s: %w", ip, err))
		}

		return
	}

	// Gather the logs
	go func() {
		if err := gatherLogs(ctx, outputter); err != nil {
			outputter.SetError(fmt.Errorf("failed to gether logs: %w", err))

			return
		}
	}()

	if err := handleSignals(ctx, closeCh); err != nil {
		outputter.SetError(fmt.Errorf("failed to handle signals: %w", err))
	}
}

func runRootchain(ctx context.Context, outputter command.OutputFormatter, closeCh chan struct{}) error {
	var err error
	if dockerClient, err = dockerclient.NewClientWithOpts(dockerclient.FromEnv); err != nil {
		return err
	}

	// target directory for the chain
	if err = common.CreateDirSafe(params.dataDir, 0700); err != nil {
		return err
	}

	image := gethConsoleImage
	if params.noConsole {
		image = gethImage
	}

	genesisFilePath := filepath.Join(params.dataDir, "genesis.json")

	var started bool
	if _, err := os.Stat(genesisFilePath); err == nil {
		started = true
	} else if errors.Is(err, os.ErrNotExist) {
		started = false
	} else {
		return err
	}

	// try to pull the image
	reader, err := dockerClient.ImagePull(ctx, image, dockertypes.ImagePullOptions{})
	if err != nil {
		return err
	}
	defer reader.Close()

	if _, err = io.Copy(outputter, reader); err != nil {
		return fmt.Errorf("cannot copy: %w", err)
	}

	if !started {
		// generate a random validator
		validatorKey, err := wallet.GenerateKey()
		if err != nil {
			return err
		}

		// generate the genesis.json file
		tmpl, err := template.New("name").Parse(genesisTmpl)
		if err != nil {
			return err
		}

		premine := map[string]string{}
		for _, addr := range params.premine {
			premine[ethgo.HexToAddress(addr).String()[2:]] = "0xffffffffffffffffffffffffff"
		}

		input := map[string]interface{}{
			"ValidatorAddr": validatorKey.Address().String()[2:],
			"Period":        2,
			"Allocs":        premine,
		}

		var tpl bytes.Buffer
		if err = tmpl.Execute(&tpl, input); err != nil {
			return err
		}

		keystore, err := toKeystoreV3(validatorKey)
		if err != nil {
			return err
		}

		// write all the data
		filesToWrite := map[string][]byte{
			"genesis.json":          tpl.Bytes(),
			"keystore/account.json": keystore,
			"password.txt":          []byte("password"),
		}
		for path, content := range filesToWrite {
			localPath := filepath.Join(params.dataDir, path)

			parentDir := filepath.Dir(localPath)
			if err := os.MkdirAll(parentDir, 0700); err != nil {
				return err
			}

			if err := common.SaveFileSafe(localPath, content, 0660); err != nil {
				return err
			}
		}
	}

	// We run two commands inside the same bash sh session of the docker container.
	// The first script initializes the data folder for Geth with a custom genesis file
	// which runs a Clique consensus protocol with premine accounts.
	// The second script, runs the Geth server with the initialized chain. Unlike in dev mode,
	// it needs to manually activate the mining module.
	args := []string{
		// init with a custom genesis
		"geth",
		"--datadir", "/eth1data",
		"init", "/eth1data/genesis.json",
		"&&",
		// start the execution node
		"geth",
		"--datadir", "/eth1data",
		"--networkid", "1337",
		"--ipcpath", "/eth1data/geth.ipc",
		"--http", "--http.addr", "0.0.0.0", "--http.api", "eth,net,web3,debug",
		"--ws", "--ws.addr", "0.0.0.0",
		"--keystore", "/eth1data/keystore",
		"--mine",
		"--miner.threads", "1",
		//nolint:godox
		// TODO: Unlock by index might be deprecated in future versions.
		// As of now, we do not need a new release of geth to run the rootchain for debug.
		// So, we can keep this for now.
		"--unlock", "0",
		"--allow-insecure-unlock",
		"--password", "/eth1data/password.txt",
	}

	config := &container.Config{
		Image:      image,
		Cmd:        []string{strings.Join(args, " ")},
		Entrypoint: []string{"/bin/sh", "-c"},
		Labels: map[string]string{
			"edge-type": "rootchain",
		},
	}

	mountDir := params.dataDir

	// we need to use the full path
	if !strings.HasPrefix(params.dataDir, "/") {
		// if the path is not absolute, assume we want to create it locally
		// in current folder
		pwdDir, err := os.Getwd()
		if err != nil {
			log.Fatal(err)
		} else {
			mountDir = filepath.Join(pwdDir, params.dataDir)
		}
	}

	port := nat.Port(fmt.Sprintf("%s/tcp", defaultHostPort))
	hostConfig := &container.HostConfig{
		Binds: []string{
			mountDir + ":/eth1data",
		},
		PortBindings: nat.PortMap{
			port: []nat.PortBinding{
				{
					HostIP:   defaultHostIP,
					HostPort: defaultHostPort,
				},
			},
		},
	}

	resp, err := dockerClient.ContainerCreate(ctx, config, hostConfig, nil, nil, "")
	if err != nil {
		return err
	}

	// start the client
	if err = dockerClient.ContainerStart(ctx, resp.ID, dockertypes.ContainerStartOptions{}); err != nil {
		return err
	}

	dockerContainerID = resp.ID

	// wait for it to finish
	go func() {
		statusCh, errCh := dockerClient.ContainerWait(ctx, dockerContainerID, container.WaitConditionNotRunning)
		select {
		case err = <-errCh:
			outputter.SetError(err)
		case status := <-statusCh:
			outputter.SetCommandResult(newContainerStopResult(status))
		}
		close(closeCh)
	}()

	return nil
}

func gatherLogs(ctx context.Context, outputter command.OutputFormatter) error {
	opts := dockertypes.ContainerLogsOptions{
		ShowStderr: true,
		ShowStdout: true,
		Follow:     true,
	}

	out, err := dockerClient.ContainerLogs(ctx, dockerContainerID, opts)
	if err != nil {
		return fmt.Errorf("failed to retrieve container logs: %w", err)
	}

	if _, err = stdcopy.StdCopy(outputter, outputter, out); err != nil {
		return fmt.Errorf("failed to write container logs to the stdout: %w", err)
	}

	return nil
}

func PingServer(closeCh <-chan struct{}) error {
	httpTimer := time.NewTimer(30 * time.Second)
	httpClient := http.Client{
		Timeout: 5 * time.Second,
	}

	for {
		select {
		case <-time.After(500 * time.Millisecond):
			resp, err := httpClient.Post(fmt.Sprintf("http://%s:%s", defaultHostIP, defaultHostPort), "application/json", nil)
			if err == nil {
				return resp.Body.Close()
			}
		case <-httpTimer.C:
			return fmt.Errorf("timeout to start http")
		case <-closeCh:
			return fmt.Errorf("closed before connecting with http. Is there any other process running and using rootchain dir?")
		}
	}
}

func handleSignals(ctx context.Context, closeCh <-chan struct{}) error {
	signalCh := make(chan os.Signal, 4)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)

	stop := true
	select {
	case <-signalCh:
	case <-closeCh:
		stop = false
	}

	// close the container if possible
	if stop {
		if err := dockerClient.ContainerStop(ctx, dockerContainerID, nil); err != nil {
			return fmt.Errorf("failed to stop container: %w", err)
		}
	}

	return nil
}

func toKeystoreV3(key *wallet.Key) ([]byte, error) {
	id, _ := uuid.GenerateUUID()

	// keystore does not include "address" and "id" field
	privKey, err := key.MarshallPrivateKey()
	if err != nil {
		return nil, err
	}

	keystore, err := keystore.EncryptV3(privKey, "password")
	if err != nil {
		return nil, err
	}

	var dec map[string]interface{}
	if err := json.Unmarshal(keystore, &dec); err != nil {
		return nil, err
	}

	//nolint:godox
	// TODO: This fields are not populated by default by
	// ethgo/keystore so we have to go through this loop to do it.
	dec["address"] = key.Address().String()
	dec["uuid"] = id
	dec["id"] = id

	return json.Marshal(dec)
}
