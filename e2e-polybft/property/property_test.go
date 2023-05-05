package property

import (
	"fmt"
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

func TestProperty_DifferentVotingPower(t *testing.T) {
	t.Parallel()

	const (
		blockTime  = time.Second * 6
		maxPremine = math.MaxUint64
	)

	rapid.Check(t, func(tt *rapid.T) {
		var (
			numNodes  = rapid.Uint64Range(5, 8).Draw(tt, "number of cluster nodes")
			epochSize = rapid.OneOf(rapid.Just(4), rapid.Just(10)).Draw(tt, "epoch size")
			numBlocks = rapid.Uint64Range(2, 5).Draw(tt, "number of blocks the cluster should mine")
		)

		premine := make([]uint64, numNodes)

		// premined amount will determine validator's stake and therefore voting power
		for i := range premine {
			premine[i] = rapid.Uint64Range(1, maxPremine).Draw(tt, fmt.Sprintf("stake for node %d", i+1))
		}

		cluster := framework.NewPropertyTestCluster(t, int(numNodes),
			framework.WithEpochSize(epochSize),
			framework.WithSecretsCallback(func(adresses []types.Address, config *framework.TestClusterConfig) {
				for i, a := range adresses {
					config.Premine = append(config.Premine, fmt.Sprintf("%s:%d", a, premine[i]))
				}
			}))
		defer cluster.Stop()

		t.Logf("Test %v, run with %d nodes, epoch size: %d. Number of blocks to mine: %d",
			filepath.Base(cluster.Config.LogsDir), numNodes, epochSize, numBlocks)

		// wait for single epoch to process withdrawal
		require.NoError(t, cluster.WaitForBlock(numBlocks, blockTime*time.Duration(numBlocks)))
	})
}
