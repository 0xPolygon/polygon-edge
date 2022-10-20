package helper

import (
	"context"
	"fmt"

	"github.com/0xPolygon/polygon-edge/types"
	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
)

var (
	// StateSenderAddress is an address of StateSender.sol smart contract
	StateSenderAddress = types.StringToAddress("0x6FE03c2768C9d800AF3Dedf1878b5687FE120a27")
	// CheckpointManagerAddress is an address of CheckpointManager.sol smart contract
	CheckpointManagerAddress = types.StringToAddress("0x3d46A809D5767B81a8836f0E79145ba615A2Dd61")
	// RootValidatorSetAddress is an address of RootValidatorSet.sol smart contract
	RootValidatorSetAddress = types.StringToAddress("0x72E1C51FE6dABF2e3d5701170cf5aD3620E6B8ba")
	// BLSAddress is an address of BLS.sol smart contract
	BLSAddress = types.StringToAddress("0x436604426F31A05f905C64edc973E575BdB46471")
	// BN256G2Address is an address of BN256G2Address.sol smart contract
	BN256G2Address = types.StringToAddress("0x947a581B2713F58A8145201DA41BCb6aAE90196B")
)

func GetRootchainID() string {
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		panic(err)
	}

	containers, err := cli.ContainerList(context.Background(), dockertypes.ContainerListOptions{})
	if err != nil {
		panic(err)
	}

	var contID string

	for _, c := range containers {
		if c.Labels["edge-type"] == "rootchain" {
			contID = c.ID

			break
		}
	}

	return contID
}

func ReadRootchainIP() string {
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		panic(err)
	}

	contID := GetRootchainID()
	if contID == "" {
		panic("container not found")
	}

	inspect, err := cli.ContainerInspect(context.Background(), contID)
	if err != nil {
		panic(err)
	}

	ports, ok := inspect.HostConfig.PortBindings["8545/tcp"]
	if !ok || len(ports) == 0 {
		panic("port 8545 is not bind with localhost")
	}

	return fmt.Sprintf("http://%s:%s", ports[0].HostIP, ports[0].HostPort)
}
