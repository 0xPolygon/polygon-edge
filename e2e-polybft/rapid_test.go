package e2e

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
	"github.com/0xPolygon/polygon-edge/types"
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
			numNodes  = rapid.Uint64Range(4, 30).Draw(tt, "number of cluster nodes")
			epochSize = rapid.IntRange(5, 20).Draw(tt, "Size of epoch")
			numBlocks = rapid.Uint64Range(2, 40).Draw(tt, "number of block the cluster should mine")
		)

		premine := make([]uint64, numNodes)

		// premined amount will determine validator's stake and in the end voting power
		for i := range premine {
			premine[i] = rapid.Uint64Range(1, maxPremine).Draw(tt, fmt.Sprintf("stake for node %d", i+1))
		}

		cluster := framework.NewTestCluster(t, int(numNodes),
			framework.WithEpochSize(epochSize),
			framework.WithSecretsCallback(func(adresses []types.Address, config *framework.TestClusterConfig) {
				for i, a := range adresses {
					config.Premine = append(config.Premine, fmt.Sprintf("%s:%d", a, premine[i]))
				}
			}),
			framework.WithNonValidators(2), framework.WithValidatorSnapshot(5))
		defer cluster.Stop()

		// wait for single epoch to process withdrawal
		cluster.WaitForBlock(numBlocks, blockTime*time.Duration(numBlocks))
	})
}
