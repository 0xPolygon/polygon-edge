package helper

import (
	"context"
	"errors"
	"fmt"
	"github.com/umbracle/ethgo"

	"github.com/0xPolygon/polygon-edge/types"
	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
)

var (
	// StateSenderAddress is an address of StateSender.sol smart contract
	StateSenderAddress = types.StringToAddress("0x6FE03c2768C9d800AF3Dedf1878b5687FE120a27")
	// CheckpointManagerAddress is an address of CheckpointManager.sol smart contract
	CheckpointManagerAddress = types.StringToAddress("0x3d46A809D5767B81a8836f0E79145ba615A2Dd61")
	// BLSAddress is an address of BLS.sol smart contract
	BLSAddress = types.StringToAddress("0x72E1C51FE6dABF2e3d5701170cf5aD3620E6B8ba")
	// BN256G2Address is an address of BN256G2Address.sol smart contract
	BN256G2Address = types.StringToAddress("0x436604426F31A05f905C64edc973E575BdB46471")
	// ExitHelperAddress is an address of ExitHelper.sol smart contract
	ExitHelperAddress = ethgo.Address(types.StringToAddress("0x947a581B2713F58A8145201DA41BCb6aAE90196B"))

	ErrRootchainNotFound = errors.New("rootchain not found")
	ErrRootchainPortBind = errors.New("port 8545 is not bind with localhost")
)

func GetRootchainID() (string, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return "", fmt.Errorf("rootchain id error: %w", err)
	}

	containers, err := cli.ContainerList(context.Background(), dockertypes.ContainerListOptions{})
	if err != nil {
		return "", fmt.Errorf("rootchain id error: %w", err)
	}

	for _, c := range containers {
		if c.Labels["edge-type"] == "rootchain" {
			return c.ID, nil
		}
	}

	return "", ErrRootchainNotFound
}

func ReadRootchainIP() (string, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return "", fmt.Errorf("rootchain id error: %w", err)
	}

	contID, err := GetRootchainID()
	if err != nil {
		return "", err
	}

	inspect, err := cli.ContainerInspect(context.Background(), contID)
	if err != nil {
		return "", fmt.Errorf("rootchain ip error: %w", err)
	}

	ports, ok := inspect.HostConfig.PortBindings["8545/tcp"]
	if !ok || len(ports) == 0 {
		return "", ErrRootchainPortBind
	}

	return fmt.Sprintf("http://%s:%s", ports[0].HostIP, ports[0].HostPort), nil
}
