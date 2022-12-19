package tests

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	dockerclient "github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
)

const (
	gethConsoleImage = "ghcr.io/0xpolygon/go-ethereum-console:latest"
	gethImage        = "ethereum/client-go:v1.9.25"

	defaultHostIP   = "127.0.0.1"
	defaultHostPort = "8545"
)

var (
	dockerClient      *dockerclient.Client
	dockerContainerID string
)

type containerStopResult struct {
	Status int64  `json:"status"`
	Err    string `json:"err"`
}

func newContainerStopResult(status container.ContainerWaitOKBody) *containerStopResult {
	var errMsg string
	if status.Error != nil {
		errMsg = status.Error.Message
	}

	return &containerStopResult{
		Status: status.StatusCode,
		Err:    errMsg,
	}
}

func RunRootchain(ctx context.Context, dataDir string, closeCh chan struct{}) error {
	var err error
	if dockerClient, err = dockerclient.NewClientWithOpts(dockerclient.FromEnv); err != nil {
		return err
	}

	// target directory for the chain
	if err = os.MkdirAll("./rootchain", 0700); err != nil {
		return err
	}

	// try to pull the image
	reader, err := dockerClient.ImagePull(ctx, gethImage, dockertypes.ImagePullOptions{})
	if err != nil {
		return err
	}
	defer reader.Close()

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
		Image: gethImage,
		Cmd:   args,
		Labels: map[string]string{
			"edge-type": "rootchain",
		},
	}

	mountDir := dataDir

	// we need to use the full path
	if !strings.HasPrefix(dataDir, "/") {
		// if the path is not absolute, assume we want to create it locally
		// in current folder
		pwdDir, err := os.Getwd()
		if err != nil {
			log.Fatal(err)
		} else {
			mountDir = filepath.Join(pwdDir, dataDir)
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
			fmt.Println(err)
		case status := <-statusCh:
			fmt.Println(status)
		}
		close(closeCh)
	}()

	return nil
}
