package server

import (
	"context"
	"fmt"
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
	"github.com/spf13/cobra"
	"github.com/umbracle/ethgo"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/rootchain/helper"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/polybftcontracts"
	"github.com/0xPolygon/polygon-edge/types"
)

const (
	imageName       = "ethereum/client-go"
	imageTag        = "v1.9.25"
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
	if containerID := helper.GetRootchainID(); containerID != "" {
		outputter.SetError(fmt.Errorf("rootchain already running: %s", containerID))

		return
	}

	// Start the client
	if err := runRootchain(ctx, outputter, closeCh); err != nil {
		outputter.SetError(fmt.Errorf("failed to run rootchain: %w", err))

		return
	}

	// Ping geth server to make sure everything is up and running
	if err := pingServer(closeCh); err != nil {
		close(closeCh)
		outputter.SetError(fmt.Errorf("failed to ping rootchain server at address %s: %w", helper.ReadRootchainIP(), err))

		return
	}

	// perform any initial deploy  of contracts
	if err := initialDeploy(outputter); err != nil {
		close(closeCh)
		outputter.SetError(fmt.Errorf("failed to deploy: %w", err))

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
	if err = os.MkdirAll(params.dataDir, 0700); err != nil {
		return err
	}

	// try to pull the image
	reader, err := dockerClient.ImagePull(ctx, "docker.io/"+imageName+":"+imageTag, dockertypes.ImagePullOptions{})
	if err != nil {
		return err
	}
	defer reader.Close()

	if _, err = io.Copy(outputter, reader); err != nil {
		return fmt.Errorf("cannot copy: %w", err)
	}

	// create the client
	args := []string{"--dev"}

	// add period of 2 seconds
	args = append(args, "--dev.period", "2")

	// add data dir
	args = append(args, "--datadir", "/eth1data")

	// add ipcpath
	args = append(args, "--ipcpath", "/eth1data/geth.ipc")

	// enable rpc
	args = append(args, "--http", "--http.addr", "0.0.0.0", "--http.api", "eth,net,web3,debug")

	// enable ws
	args = append(args, "--ws", "--ws.addr", "0.0.0.0")

	config := &container.Config{
		Image: imageName + ":" + imageTag,
		Cmd:   args,
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
		AutoRemove: true,
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

func initialDeploy(outputter command.OutputFormatter) error {
	// if the bridge contract is not created, we have to deploy all the contracts
	if helper.ExistsCode(helper.RootchainBridgeAddress) {
		return nil
	}

	// fund account
	if _, err := helper.FundAccount(helper.GetDefAccount()); err != nil {
		return err
	}

	deployContracts := map[string]types.Address{
		"RootchainBridge": helper.RootchainBridgeAddress,
		"Checkpoint":      helper.CheckpointManagerAddress,
	}

	for name, address := range deployContracts {
		artifact := polybftcontracts.MustReadArtifact("rootchain", name)

		input, err := artifact.DeployInput(nil)
		if err != nil {
			return err
		}

		txn := &ethgo.Transaction{
			To:    nil, // contract deployment
			Input: input,
		}

		pendingNonce, err := helper.GetPendingNonce(helper.GetDefAccount())
		if err != nil {
			return err
		}

		receipt, err := helper.SendTxn(pendingNonce, txn)
		if err != nil {
			return err
		}

		if types.Address(receipt.ContractAddress) != address {
			return fmt.Errorf("wrong deployed address for %s: expected %s but found %s", name, address, receipt.ContractAddress)
		}

		outputter.WriteCommandResult(newInitialDeployResult(name, address, receipt.TransactionHash))
	}

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

func pingServer(closeCh <-chan struct{}) error {
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
