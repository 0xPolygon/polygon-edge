package property

import (
	"fmt"
	"math/big"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo/wallet"
	"pgregory.net/rapid"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
	"github.com/0xPolygon/polygon-edge/types"
)

func TestProperty_DifferentVotingPower(t *testing.T) {
	const (
		blockTime = time.Second * 6
		maxStake  = 20
	)

	minter, err := wallet.GenerateKey()
	require.NoError(t, err)

	rapid.Check(t, func(tt *rapid.T) {
		var (
			numNodes  = rapid.Uint64Range(5, 8).Draw(tt, "number of cluster nodes")
			epochSize = rapid.OneOf(rapid.Just(4), rapid.Just(10)).Draw(tt, "epoch size")
			numBlocks = rapid.Uint64Range(2, 5).Draw(tt, "number of blocks the cluster should mine")
		)

		stakes := make([]*big.Int, numNodes)

		// stake amount will determine validator's stake and therefore voting power
		for i := range stakes {
			stakes[i] = new(big.Int).
				SetUint64(rapid.Uint64Range(1, maxStake).
					Draw(tt, fmt.Sprintf("stake for node %d", i+1)))
		}

		cluster := framework.NewPropertyTestCluster(t, int(numNodes),
			framework.WithEpochSize(epochSize),
			framework.WithBlockTime(blockTime),
			framework.WithNativeTokenConfig(fmt.Sprintf(framework.NativeTokenMintableTestCfg, minter.Address())),
			framework.WithSecretsCallback(func(adresses []types.Address, config *framework.TestClusterConfig) {
				for i := range adresses {
					config.StakeAmounts = append(config.StakeAmounts, stakes[i])
				}
			}))
		defer cluster.Stop()

		t.Logf("Test %v, run with %d nodes, epoch size: %d. Number of blocks to mine: %d",
			filepath.Base(cluster.Config.LogsDir), numNodes, epochSize, numBlocks)

		// wait for single epoch to process withdrawal
		require.NoError(t, cluster.WaitForBlock(numBlocks, 2*blockTime*time.Duration(numBlocks)))
	})
}

func TestProperty_DropValidators(t *testing.T) {
	const (
		blockTime = time.Second * 4
	)

	minter, err := wallet.GenerateKey()
	require.NoError(t, err)

	rapid.Check(t, func(tt *rapid.T) {
		var (
			numNodes  = rapid.Uint64Range(5, 8).Draw(tt, "number of cluster nodes")
			epochSize = rapid.OneOf(rapid.Just(4), rapid.Just(10)).Draw(tt, "epoch size")
		)

		cluster := framework.NewPropertyTestCluster(t, int(numNodes),
			framework.WithEpochSize(epochSize),
			framework.WithBlockTime(blockTime),
			framework.WithNativeTokenConfig(fmt.Sprintf(framework.NativeTokenMintableTestCfg, minter.Address())),
			framework.WithSecretsCallback(func(adresses []types.Address, config *framework.TestClusterConfig) {
				for range adresses {
					config.StakeAmounts = append(config.StakeAmounts, big.NewInt(20))
				}
			}))
		defer cluster.Stop()

		t.Logf("Test %v, run with %d nodes, epoch size: %d",
			filepath.Base(cluster.Config.LogsDir), numNodes, epochSize)

		cluster.WaitForReady(t)

		// stop first validator, block production should continue
		cluster.Servers[0].Stop()
		activeValidator := cluster.Servers[numNodes-1]
		currentBlock, err := activeValidator.JSONRPC().Eth().BlockNumber()
		require.NoError(t, err)
		require.NoError(t, cluster.WaitForBlock(currentBlock+1, 2*blockTime))

		// drop all validator nodes, leaving one node alive
		numNodesToDrop := int(numNodes - 1)

		var wg sync.WaitGroup
		// drop bulk of nodes from cluster
		for i := 1; i < numNodesToDrop; i++ {
			node := cluster.Servers[i]

			wg.Add(1)

			go func(node *framework.TestServer) {
				defer wg.Done()
				node.Stop()
			}(node)
		}

		wg.Wait()

		// check that block production is stoped
		currentBlock, err = activeValidator.JSONRPC().Eth().BlockNumber()
		require.NoError(t, err)
		oldBlockNumber := currentBlock
		time.Sleep(2 * blockTime)
		currentBlock, err = activeValidator.JSONRPC().Eth().BlockNumber()
		require.NoError(t, err)
		require.Equal(t, oldBlockNumber, currentBlock)

		// start dropped nodes again
		for i := 0; i < numNodesToDrop; i++ {
			node := cluster.Servers[i]
			node.Start()
		}

		require.NoError(t, cluster.WaitForBlock(oldBlockNumber+1, 3*blockTime))
	})
}
