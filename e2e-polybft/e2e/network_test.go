package e2e

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/types/known/emptypb"
)

func init() {
	wd, err := os.Getwd()
	if err != nil {
		return
	}

	parent := filepath.Dir(wd)
	parent = strings.Trim(parent, "e2e-polybft")
	wd = filepath.Join(parent, "/artifacts/blade")
	os.Setenv("EDGE_BINARY", wd)
	os.Setenv("E2E_TESTS", "true")
	os.Setenv("E2E_LOGS", "true")
	os.Setenv("E2E_LOG_LEVEL", "debug")
}

func TestE2E_NetworkDiscoveryProtocol(t *testing.T) {
	const (
		validatorCount    = 5
		nonValidatorCount = 5
		// each node in cluster should find at least 2 more peers beside bootnode
		atLeastPeers = 3
		testTimeout  = time.Second * 60
	)

	// create cluster
	cluster := framework.NewTestCluster(t, validatorCount,
		framework.WithNonValidators(nonValidatorCount),
		framework.WithBootnodeCount(1))
	defer cluster.Stop()

	ctx := context.Background()

	// wait for everyone to have at least 'atLeastPeers' peers
	err := cluster.WaitForGeneric(testTimeout, func(ts *framework.TestServer) bool {
		peerList, err := ts.Conn().PeersList(ctx, &emptypb.Empty{})

		return err == nil && len(peerList.GetPeers()) >= atLeastPeers
	})
	assert.NoError(t, err)
}
