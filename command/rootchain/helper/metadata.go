package helper

import (
	"context"
	"fmt"

	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/client"

	"github.com/0xPolygon/polygon-edge/types"
)

var (
	RootchainBridgeAddress   = types.StringToAddress("0x6FE03c2768C9d800AF3Dedf1878b5687FE120a27")
	SidechainBridgeAddr      = types.StringToAddress("0x8Be503bcdEd90ED42Eff31f56199399B2b0154CA")
	CheckpointManagerAddress = types.StringToAddress("0x3d46A809D5767B81a8836f0E79145ba615A2Dd61")
)

func GetRootchainID() string {
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		panic(err)
	}

	ctx := context.Background()
	containers, err := cli.ContainerList(ctx, dockertypes.ContainerListOptions{})
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
