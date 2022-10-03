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
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-connections/nat"
	"github.com/spf13/cobra"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/jsonrpc"

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
	params serverParams
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

	c.logger = log.New(os.Stdout, "", 0)

	// check if the client is already running
	if containerId := helper.GetRootchainID(); containerId != "" {
		outputter.SetError(fmt.Errorf("rootchain already running: %s", containerId))

		return
	}

	// start the client
	if err := runRootchain(); err != nil {
		outputter.SetError(fmt.Errorf("failed to run rootchain: %s", err))

		return
	}

	if err := PingServer(c.closeCh); err != nil {
		close(c.closeCh)
		outputter.SetError(fmt.Errorf("Failed to ping rootchain server at address %s", helper.ReadRootchainIP()))

		return
	}

	// gather the logs
	go gatherLogs()

	// perform any initial deploy on parallel
	go func() {
		if err := initialDeploy(); err != nil {
			outputter.SetError(fmt.Errorf("failed to deploy: %v", err))

			return
		}
	}()

	return handleSignals()
}

func handleSignals() int {
	signalCh := make(chan os.Signal, 4)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)

	stop := true
	select {
	case sig := <-signalCh:
		c.UI.Output(fmt.Sprintf("Caught signal: %v", sig))
		c.UI.Output("Gracefully shutting down rootchain server...")
	case <-c.closeCh:
		stop = false
	}

	// close the container if possible
	if stop {
		ctx := context.Background()

		if err := c.client.ContainerStop(ctx, c.id, nil); err != nil {
			c.UI.Error(fmt.Sprintf("Failed to stop container: %v", err))
		}
	}

	return 0
}

func gatherLogs() {
	ctx := context.Background()

	opts := dockertypes.ContainerLogsOptions{
		ShowStderr: true,
		ShowStdout: true,
		Follow:     true,
	}
	out, err := c.client.ContainerLogs(ctx, c.id, opts)
	if err != nil {
		panic(fmt.Errorf("Failed to retrieve container logs. Error: %v", err))
	}

	_, err = stdcopy.StdCopy(os.Stdout, os.Stderr, out)
	if err != nil {
		panic(fmt.Errorf("Failed to write container logs to the stdout. Error: %v", err))
	}

	fmt.Println("Docker container logs retrieval done")
}

func runRootchain() error {
	c.closeCh = make(chan struct{})

	ctx := context.Background()

	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return err
	}
	c.client = cli

	// target directory for the chain
	if err = os.MkdirAll(params.dataDir, 0700); err != nil {
		return err
	}

	// try to pull the image
	reader, err := cli.ImagePull(ctx, "docker.io/"+imageName+":"+imageTag, dockertypes.ImagePullOptions{})
	if err != nil {
		return err
	}

	_, err = io.Copy(c.logger.Writer(), reader)
	if err != nil {
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
	resp, err := cli.ContainerCreate(ctx, config, hostConfig, nil, nil, "")
	if err != nil {
		return err
	}

	// start the client
	if err = cli.ContainerStart(ctx, resp.ID, dockertypes.ContainerStartOptions{}); err != nil {
		return err
	}
	c.id = resp.ID
	c.logger.Printf("Container started: id, %s", resp.ID)

	// wait for it to finish
	go func() {
		statusCh, errCh := c.client.ContainerWait(context.Background(), c.id, container.WaitConditionNotRunning)
		select {
		case err = <-errCh:
			c.UI.Error(fmt.Sprintf("failed to wait for container: %s", err))
		case status := <-statusCh:
			c.UI.Output(fmt.Sprintf("Done with status %d", status.StatusCode))
		}
		close(c.closeCh)
	}()

	return nil
}

func PingServer(closeCh <-chan struct{}) error {
	if closeCh == nil {
		closeCh = make(chan struct{})
	}
	httpTimer := time.NewTimer(30 * time.Second)
	httpClient := http.Client{
		Timeout: 5 * time.Second,
	}

	for {
		select {
		case <-time.After(500 * time.Millisecond):
			resp, err := httpClient.Post(fmt.Sprintf("http://%s:%s", defaultHostIP, defaultHostPort), "application/json", nil)
			if err == nil {
				resp.Body.Close()
				return nil
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
	if helper.ExistsCode(ethgo.Address(helper.RootchainBridgeAddress)) {
		return nil
	}

	// fund account
	if err := helper.FundAccount(helper.GetDefAccount()); err != nil {
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

	ipAddr := helper.ReadRootchainIP()
	rpcClient, err := jsonrpc.NewClient(ipAddr)
	if err != nil {
		return err
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

		pendingNonce, err := rpcClient.Eth().GetNonce(helper.GetDefAccount(), ethgo.Pending)
		if err != nil {
			return err
		}

		receipt, err := helper.SendTxn(rpcClient, pendingNonce, txn)
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
