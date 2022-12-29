package e2e

import (
	"math/big"
	"path"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/command/genesis"
	"github.com/0xPolygon/polygon-edge/command/sidechain"
	"github.com/0xPolygon/polygon-edge/consensus/polybft"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
)

func TestE2E_Consensus_Basic_WithNonValidators(t *testing.T) {
	cluster := framework.NewTestCluster(t, 7,
		framework.WithNonValidators(2), framework.WithValidatorSnapshot(5))
	defer cluster.Stop()

	require.NoError(t, cluster.WaitForBlock(22, 1*time.Minute))
}

func TestE2E_Consensus_Sync_WithNonValidators(t *testing.T) {
	// one non-validator node from the ensemble gets disconnected and connected again.
	// It should be able to pick up from the synchronization protocol again.
	cluster := framework.NewTestCluster(t, 7,
		framework.WithNonValidators(2), framework.WithValidatorSnapshot(5))
	defer cluster.Stop()

	// wait for the start
	require.NoError(t, cluster.WaitForBlock(20, 1*time.Minute))

	// stop one non-validator node
	node := cluster.Servers[6]
	node.Stop()

	// wait for at least 15 more blocks before starting again
	require.NoError(t, cluster.WaitForBlock(35, 2*time.Minute))

	// start the node again
	node.Start()

	// wait for block 55
	require.NoError(t, cluster.WaitForBlock(55, 2*time.Minute))
}

func TestE2E_Consensus_Sync(t *testing.T) {
	// one node from the ensemble gets disconnected and connected again.
	// It should be able to pick up from the synchronization protocol again.
	cluster := framework.NewTestCluster(t, 6, framework.WithValidatorSnapshot(6))
	defer cluster.Stop()

	// wait for the start
	require.NoError(t, cluster.WaitForBlock(5, 1*time.Minute))

	// stop one node
	node := cluster.Servers[0]
	node.Stop()

	// wait for at least 15 more blocks before starting again
	require.NoError(t, cluster.WaitForBlock(20, 2*time.Minute))

	// start the node again
	node.Start()

	// wait for block 35
	require.NoError(t, cluster.WaitForBlock(35, 2*time.Minute))
}

func TestE2E_Consensus_Bulk_Drop(t *testing.T) {
	clusterSize := 5
	bulkToDrop := 3

	cluster := framework.NewTestCluster(t, clusterSize)
	defer cluster.Stop()

	// wait for cluster to start
	require.NoError(t, cluster.WaitForBlock(5, 1*time.Minute))

	// drop bulk of nodes from cluster
	for i := 0; i < bulkToDrop; i++ {
		node := cluster.Servers[i]
		node.Stop()
	}

	// start dropped nodes again
	for i := 0; i < bulkToDrop; i++ {
		node := cluster.Servers[i]
		node.Start()
	}

	// wait for block 10
	require.NoError(t, cluster.WaitForBlock(10, 2*time.Minute))
}

func TestE2E_Consensus_RegisterValidator(t *testing.T) {
	const (
		validatorSize       = 5
		newValidatorSecrets = "test-chain-6"
	)

	cluster := framework.NewTestCluster(t, validatorSize,
		framework.WithEpochSize(5),
		framework.WithEpochReward(1000))

	srv := cluster.Servers[0]

	// create new account
	require.NoError(t, cluster.InitSecrets(newValidatorSecrets, 1))

	// assert that account is created
	validators, err := genesis.GetValidatorKeyFiles(cluster.Config.TmpDir, cluster.Config.ValidatorPrefix)
	require.NoError(t, err)
	require.Equal(t, validatorSize+1, len(validators))

	// wait for consensus to start
	require.NoError(t, cluster.WaitForBlock(1, 10*time.Second))

	// register new validator
	require.NoError(t, srv.RegisterValidator(newValidatorSecrets))

	// wait for two epochs so that stake gets settled
	cluster.WaitForBlock(11, 1*time.Minute)

	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(srv.JSONRPCAddr()))
	require.NoError(t, err)

	newValidatorAcc, err := sidechain.GetAccountFromDir(path.Join(cluster.Config.TmpDir, validators[len(validators)-1]))
	require.NoError(t, err)

	validatorInfoRaw, err := sidechain.GetValidatorInfo(newValidatorAcc.Ecdsa.Address(), txRelayer)
	require.NoError(t, err)

	// cluster.WaitForBlock(20, 1*time.Minute)
	require.Equal(t, uint64(1000), validatorInfoRaw["totalStake"].(*big.Int).Uint64()) //nolint:forcetypeassert

	block, err := srv.JSONRPC().Eth().GetBlockByNumber(ethgo.Latest, false)
	require.NoError(t, err)

	extra, err := polybft.GetIbftExtra(block.ExtraData)
	require.NoError(t, err)
	require.NotNil(t, extra.Checkpoint)

	systemState := polybft.NewSystemState(
		&polybft.PolyBFTConfig{
			StateReceiverAddr: contracts.StateReceiverContract,
			ValidatorSetAddr:  contracts.ValidatorSetContract},
		&e2eStateProvider{txRelayer: txRelayer})

	accountSet, err := systemState.GetValidatorSet()
	require.NoError(t, err)

	t.Logf("validators: %v\n", accountSet)

	require.Equal(t, accountSet.ContainsAddress(types.Address(newValidatorAcc.Ecdsa.Address())), true)

	accHash, err := accountSet.Hash()
	require.NoError(t, err)

	require.Equal(t, extra.Checkpoint.NextValidatorsHash, accHash)

	cluster.InitTestServer(t, 6, true)

	cluster.WaitForBlock(30, 2*time.Minute)

	validatorInfoRaw, err = sidechain.GetValidatorInfo(newValidatorAcc.Ecdsa.Address(), txRelayer)
	// t.Log("Stake:", validatorInfoRaw["stake"].(*big.Int), "Total stake:", validatorInfoRaw["totalStake"].(*big.Int), "Reward:", validatorInfoRaw["withdrawableRewards"].(*big.Int))
	require.NoError(t, err)
}
