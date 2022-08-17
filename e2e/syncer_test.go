package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/e2e/framework"
)

func TestClusterBlockSync(t *testing.T) {
	const (
		numNonValidators = 2
		desiredHeight    = 10
	)

	// Start IBFT cluster (4 Validator + 2 Non-Validator)
	ibftManager := framework.NewIBFTServersManager(
		t,
		IBFTMinNodes+numNonValidators,
		IBFTDirPrefix, func(i int, config *framework.TestServerConfig) {
			if i >= IBFTMinNodes {
				// Other nodes should not be in the validator set
				dirPrefix := "polygon-edge-non-validator-"
				config.SetIBFTDirPrefix(dirPrefix)
				config.SetIBFTDir(fmt.Sprintf("%s%d", dirPrefix, i))
			}
			config.SetSeal(i < IBFTMinNodes)
		})

	startContext, startCancelFn := context.WithTimeout(context.Background(), time.Minute)
	defer startCancelFn()
	ibftManager.StartServers(startContext)

	servers := make([]*framework.TestServer, 0)
	for i := 0; i < IBFTMinNodes+numNonValidators; i++ {
		servers = append(servers, ibftManager.GetServer(i))
	}
	// All nodes should have mined the same block eventually
	waitErrors := framework.WaitForServersToSeal(servers, desiredHeight)

	if len(waitErrors) != 0 {
		t.Fatalf("Unable to wait for all nodes to seal blocks, %v", waitErrors)
	}
}
