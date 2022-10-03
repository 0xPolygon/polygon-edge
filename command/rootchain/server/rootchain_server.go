package server

import (
	"bytes"
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
	smartcontracts "github.com/0xPolygon/polygon-edge/contracts/smart_contracts"
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
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	closeCh := make(chan struct{})
	c.logger = log.New(os.Stdout, "", 0)

	// check if the client is already running
	if containerId := helper.GetRootchainID(); containerId != "" {
		outputter.SetError(fmt.Errorf("rootchain already running: %s", containerId))
		return
	}

	// start the client
	if err := runRootchain(closeCh); err != nil {
		outputter.SetError(fmt.Errorf("failed to run rootchain: %s", err))
		return
	}

	if err := pingServer(closeCh); err != nil {
		close(closeCh)
		outputter.SetError(fmt.Errorf("Failed to ping rootchain server at address %s", helper.ReadRootchainIP()))

		return
	}

	// gather the logs
	var logsOut, logsErr bytes.Buffer
	go func() {
		if err := gatherLogs(&logsOut, &logsErr); err != nil {
			outputter.SetError(fmt.Errorf("failed to gether logs: %v", err))
			return
		}
	}()

	// perform any initial deploy on parallel
	go func() {
		if err := initialDeploy(); err != nil {
			outputter.SetError(fmt.Errorf("failed to deploy: %v", err))
			return
		}
	}()

	if err := handleSignals(closeCh); err != nil {
		outputter.SetError(fmt.Errorf("failed to handle signals: %v", err))
	}
}

func runRootchain(closeCh chan struct{}) error {
	ctx := context.Background()

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

	if _, err = io.Copy(c.logger.Writer(), reader); err != nil {
		return fmt.Errorf("cannot copy:%w", err)
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
			"v3-type": "rootchain",
		},
	}

	// we need to use the full path
	mountDir := params.dataDir
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
	c.logger.Printf("Container started: id, %s", resp.ID)

	// wait for it to finish
	go func() {
		statusCh, errCh := dockerClient.ContainerWait(context.Background(), dockerContainerID, container.WaitConditionNotRunning)
		select {
		case err = <-errCh:
			c.UI.Error(fmt.Sprintf("failed to wait for container: %s", err))
		case status := <-statusCh:
			c.UI.Output(fmt.Sprintf("Done with status %d", status.StatusCode))
		}
		close(closeCh)
	}()

	return nil
}

func handleSignals(closeCh <-chan struct{}) error {
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
		ctx := context.Background()

		if err := dockerClient.ContainerStop(ctx, dockerContainerID, nil); err != nil {
			return fmt.Errorf("failed to stop container: %v", err)
		}
	}

	return nil
}

func gatherLogs(stdOut, stdErr io.Writer) error {
	ctx := context.Background()

	opts := dockertypes.ContainerLogsOptions{
		ShowStderr: true,
		ShowStdout: true,
		Follow:     true,
	}

	out, err := dockerClient.ContainerLogs(ctx, dockerContainerID, opts)
	if err != nil {
		return fmt.Errorf("failed to retrieve container logs: %v", err)
	}

	if _, err = stdcopy.StdCopy(stdOut, stdErr, out); err != nil {
		return fmt.Errorf("failed to write container logs to the stdout: %v", err)
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

func initialDeploy() error {
	// if the bridge contract is not created, we have to deploy all the contracts
	if helper.ExistsCode(helper.RootchainBridgeAddress) {
		return nil
	}

	// fund account
	if _, err := helper.FundAccount(helper.GetDefAccount()); err != nil {
		return err
	}

	deployContracts := []struct {
		name     string
		expected types.Address
	}{
		{
			name:     "RootchainBridge",
			expected: helper.RootchainBridgeAddress,
		},
		{
			name:     "Checkpoint",
			expected: helper.CheckpointManagerAddress,
		},
	}

	for _, contract := range deployContracts {
		artifact := smartcontracts.MustReadArtifact("rootchain", contract.name)

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

		if types.Address(receipt.ContractAddress) != contract.expected {
			panic(fmt.Sprintf("wrong deployed address: expected %s but found %s", contract.expected, receipt.ContractAddress))
		}

		c.UI.Output(fmt.Sprintf("Contract created: name=%s, address=%s", contract.name, contract.expected))
	}

	return nil
}
