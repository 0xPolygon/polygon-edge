package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/types/known/emptypb"
)

func TestE2E_NetworkDiscoveryProtocol(t *testing.T) {
	const (
		validatorCount    = 5
		nonValidatorCount = 5
		testTimeout       = time.Second * 60

		// each node in cluster finds at least 2 more peers beside bootnode
		peersCount = 3
	)

	// create cluster
	cluster := framework.NewTestCluster(t, 10,
		framework.WithValidatorSnapshot(validatorCount),
		framework.WithNonValidators(nonValidatorCount),
		framework.WithBootnodeCount(1))
	defer cluster.Stop()

	ctx := context.Background()

	// wait for everyone to have at least 'atLeastPeers' peers
	err := cluster.WaitForGeneric(testTimeout, func(ts *framework.TestServer) bool {
		peerList, err := ts.Conn().PeersList(ctx, &emptypb.Empty{})

		return err == nil && len(peerList.GetPeers()) >= peersCount
	})
	assert.NoError(t, err)
}
