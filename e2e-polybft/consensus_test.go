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
		epochSize           = 5
		newValidatorSecrets = "test-chain-6"
		premineBalance      = "0x1A784379D99DB42000000" // 2M native tokens (so that we have enough balance to fund new validator)
	)

	newValidatorStakeRaw := "0x152D02C7E14AF6800000"   // 100k native tokens
	newValidatorBalanceRaw := "0xD3C21BCECCEDA1000000" // 1M native tokens
	newValidatorStake, err := types.ParseUint256orHex(&newValidatorStakeRaw)
	require.NoError(t, err)

	cluster := framework.NewTestCluster(t, validatorSize,
		framework.WithEpochSize(epochSize),
		framework.WithEpochReward(1000),
		framework.WithPremineValidators(premineBalance))
	defer cluster.Stop()
	srv := cluster.Servers[0]
	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(srv.JSONRPCAddr()))
	require.NoError(t, err)

	systemState := polybft.NewSystemState(
		&polybft.PolyBFTConfig{
			StateReceiverAddr: contracts.StateReceiverContract,
			ValidatorSetAddr:  contracts.ValidatorSetContract},
		&e2eStateProvider{txRelayer: txRelayer})

	// create new account
	addrs, err := cluster.InitSecrets(newValidatorSecrets, 1)
	require.NoError(t, err)

	// extract new validator address
	newValidatorAddr := ethgo.Address(addrs[0])

	// assert that account is created
	validatorSecrets, err := genesis.GetValidatorKeyFiles(cluster.Config.TmpDir, cluster.Config.ValidatorPrefix)
	require.NoError(t, err)
	require.Equal(t, validatorSize+1, len(validatorSecrets))

	// wait for consensus to start
	require.NoError(t, cluster.WaitForBlock(1, 10*time.Second))

	// register new validator
	require.NoError(t, srv.RegisterValidator(newValidatorSecrets, newValidatorBalanceRaw, newValidatorStakeRaw))

	go func() {
		// start new validator
		cluster.InitTestServer(t, 6, true, false)
	}()

	validators := polybft.AccountSet{}
	// assert that new validator is among validator set
	require.NoError(t, cluster.WaitUntil(20*time.Second, func() bool {
		// query validators
		validators, err = systemState.GetValidatorSet()
		require.NoError(t, err)

		return validators.ContainsAddress((types.Address(newValidatorAddr)))
	}))

	// assert that validators hash is correct
	block, err := srv.JSONRPC().Eth().GetBlockByNumber(ethgo.Latest, false)
	require.NoError(t, err)
	t.Logf("Block Number=%d\n", block.Number)

	// unmarshal header extra data
	extra, err := polybft.GetIbftExtra(block.ExtraData)
	require.NoError(t, err)
	require.NotNil(t, extra.Checkpoint)

	// assert that correct validators hash gets submitted
	validatorsHash, err := validators.Hash()
	require.NoError(t, err)
	require.Equal(t, extra.Checkpoint.NextValidatorsHash, validatorsHash)

	// query registered validator
	newValidatorInfo, err := sidechain.GetValidatorInfo(newValidatorAddr, txRelayer)
	require.NoError(t, err)

	// assert registered validator's stake
	t.Logf("New validator stake=%d\n", newValidatorInfo.TotalStake)
	require.Equal(t, newValidatorStake, newValidatorInfo.TotalStake)

	// wait 3 more epochs, so that rewards get accumulated to the registered validator account
	currentBlock, err := srv.JSONRPC().Eth().BlockNumber()
	require.NoError(t, err)
	cluster.WaitForBlock(currentBlock+3*epochSize, 2*time.Minute)

	// query registered validator
	newValidatorInfo, err = sidechain.GetValidatorInfo(newValidatorAddr, txRelayer)
	require.NoError(t, err)

	// assert registered validator's rewards
	currentBlock, err = srv.JSONRPC().Eth().BlockNumber()
	require.NoError(t, err)
	t.Logf("New validator rewards (block %d)=%d\n", currentBlock, newValidatorInfo.WithdrawableRewards)
	require.True(t, newValidatorInfo.WithdrawableRewards.Cmp(big.NewInt(0)) > 0)
}

func TestE2E_Consensus_Delegation_Undelegation(t *testing.T) {
	const (
		validatorSecrets = "test-chain-1"
		delegatorSecrets = "test-chain-6"
		premineBalance   = "0x1B1AE4D6E2EF500000" // 500 native tokens (so that we have enough funds to fund delegator)
		epochSize        = 5
	)

	fundAmountRaw := "0xD8D726B7177A80000" // 250 native tokens
	fundAmount, err := types.ParseUint256orHex(&fundAmountRaw)
	require.NoError(t, err)

	cluster := framework.NewTestCluster(t, 5,
		framework.WithEpochReward(100000),
		framework.WithPremineValidators(premineBalance),
		framework.WithEpochSize(epochSize))
	defer cluster.Stop()

	// init delegator account
	_, err = cluster.InitSecrets(delegatorSecrets, 1)
	require.NoError(t, err)

	srv := cluster.Servers[0]

	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(srv.JSONRPCAddr()))
	require.NoError(t, err)

	// wait for consensus to start
	cluster.WaitForBlock(1, 10*time.Second)

	// extract delegator's secrets
	delegatorSecretsPath := path.Join(cluster.Config.TmpDir, delegatorSecrets)
	delegatorAcc, err := sidechain.GetAccountFromDir(delegatorSecretsPath)
	require.NoError(t, err)

	delegatorAddr := delegatorAcc.Ecdsa.Address()

	// extract validator's secrets
	validatorSecretsPath := path.Join(cluster.Config.TmpDir, validatorSecrets)

	validatorAcc, err := sidechain.GetAccountFromDir(validatorSecretsPath)
	require.NoError(t, err)

	validatorAddr := validatorAcc.Ecdsa.Address()

	// fund delegator
	receipt, err := txRelayer.SendTransaction(&ethgo.Transaction{
		From:  validatorAddr,
		To:    &delegatorAddr,
		Value: fundAmount,
	}, validatorAcc.Ecdsa)
	require.NoError(t, err)
	require.Equal(t, uint64(types.ReceiptSuccess), receipt.Status)

	// getDelegatorInfo queries delegator's balance and its rewards
	getDelegatorInfo := func() (balance *big.Int, reward *big.Int) {
		currentBlockNum, err := srv.JSONRPC().Eth().BlockNumber()
		require.NoError(t, err)

		balance, err = srv.JSONRPC().Eth().GetBalance(delegatorAddr, ethgo.Latest)
		require.NoError(t, err)
		t.Logf("Delegator balance (block %d)=%s\n", currentBlockNum, balance)

		reward, err = sidechain.GetDelegatorReward(validatorAddr, delegatorAddr, txRelayer)
		require.NoError(t, err)
		t.Logf("Delegator reward (block %d)=%s\n", currentBlockNum, reward)

		return
	}

	// assert that delegator received fund amount from validator
	delegatorBalance, _ := getDelegatorInfo()
	require.Equal(t, fundAmount, delegatorBalance)

	// delegate 1 native token
	delegationAmount := uint64(1e18)
	require.NoError(t, srv.Delegate(delegationAmount, delegatorSecretsPath, validatorAddr))

	// wait for 2 epochs to accumulate delegator rewards
	currentBlockNum, err := srv.JSONRPC().Eth().BlockNumber()
	require.NoError(t, err)
	cluster.WaitForBlock(currentBlockNum+2*epochSize, 1*time.Minute)

	// query delegator rewards
	_, delegatorReward := getDelegatorInfo()
	// if validator signed at leased 1 block per epoch, this will be the minimal reward for the delegator
	require.Greater(t, delegatorReward.Uint64(), uint64(16))

	// undelegate rewards
	require.NoError(t, srv.Undelegate(delegationAmount, delegatorSecretsPath, validatorAddr))
	t.Logf("Rewards are undelegated\n")

	// wait for one epoch to be able to withdraw undelegated funds
	currentBlockNum, err = srv.JSONRPC().Eth().BlockNumber()
	require.NoError(t, err)
	require.NoError(t, cluster.WaitForBlock(currentBlockNum+epochSize, time.Minute))

	balanceBeforeWithdraw, _ := getDelegatorInfo()

	// withdraw available rewards
	require.NoError(t, srv.Withdraw(delegatorSecretsPath, delegatorAddr))
	t.Logf("Funds are withdrawn\n")

	// wait for single epoch to process withdrawal
	currentBlockNum, err = srv.JSONRPC().Eth().BlockNumber()
	require.NoError(t, err)
	cluster.WaitForBlock(currentBlockNum+epochSize, time.Minute)

	// assert that delegator doesn't receive any rewards
	delegatorBalance, delegatorReward = getDelegatorInfo()
	require.True(t, delegatorReward.Cmp(big.NewInt(0)) == 0)
	require.True(t, balanceBeforeWithdraw.Cmp(delegatorBalance) == -1)
}

func TestE2E_Consensus_Validator_Unstake(t *testing.T) {
	cluster := framework.NewTestCluster(t, 5,
		framework.WithBridge(),
		framework.WithEpochReward(10000),
		framework.WithEpochSize(5),
		framework.WithPremineValidators("10000000000000000000")) // 10 native tokens
	validatorSecrets := path.Join(cluster.Config.TmpDir, "test-chain-1")
	srv := cluster.Servers[0]

	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(srv.JSONRPCAddr()))
	require.NoError(t, err)

	systemState := polybft.NewSystemState(
		&polybft.PolyBFTConfig{
			StateReceiverAddr: contracts.StateReceiverContract,
			ValidatorSetAddr:  contracts.ValidatorSetContract},
		&e2eStateProvider{txRelayer: txRelayer})

	validatorAcc, err := sidechain.GetAccountFromDir(validatorSecrets)
	require.NoError(t, err)

	validatorAddr := validatorAcc.Ecdsa.Address()

	// wait for one epoch to accumulate validator rewards
	require.NoError(t, cluster.WaitForBlock(5, 20*time.Second))

	validatorInfo, err := sidechain.GetValidatorInfo(validatorAddr, txRelayer)
	require.NoError(t, err)

	initialStake := validatorInfo.TotalStake
	t.Logf("Stake (before unstake)=%d\n", initialStake)

	// unstake entire balance (which should remove validator from the validator set in next epoch)
	require.NoError(t, srv.Unstake(initialStake.Uint64()))

	// wait end of epoch
	require.NoError(t, cluster.WaitForBlock(10, 20*time.Second))

	validatorSet, err := systemState.GetValidatorSet()
	require.NoError(t, err)

	// assert that validator isn't present in new validator set
	require.Equal(t, 4, validatorSet.Len())

	validatorInfo, err = sidechain.GetValidatorInfo(validatorAddr, txRelayer)
	require.NoError(t, err)

	reward := validatorInfo.WithdrawableRewards
	t.Logf("Rewards=%d\n", reward)
	t.Logf("Stake (after unstake)=%d\n", validatorInfo.TotalStake)
	require.Greater(t, reward.Uint64(), uint64(0))

	oldValidatorBalance, err := srv.JSONRPC().Eth().GetBalance(validatorAcc.Ecdsa.Address(), ethgo.Latest)
	require.NoError(t, err)
	t.Logf("Balance (before withdrawal)=%s\n", oldValidatorBalance)

	// withdraw (stake + rewards)
	require.NoError(t, srv.Withdraw(validatorSecrets, validatorAddr))

	newValidatorBalance, err := srv.JSONRPC().Eth().GetBalance(validatorAcc.Ecdsa.Address(), ethgo.Latest)
	require.NoError(t, err)
	t.Logf("Balance (after withdrawal)=%s\n", newValidatorBalance)
	require.True(t, newValidatorBalance.Cmp(oldValidatorBalance) > 0)

	l1Relayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(cluster.Bridge.JSONRPCAddr()))
	require.NoError(t, err)

	manifest, err := polybft.LoadManifest(path.Join(cluster.Config.TmpDir, manifestFileName))
	require.NoError(t, err)

	checkpointManagerAddr := ethgo.Address(manifest.RootchainConfig.CheckpointManagerAddress)

	// query rootchain validator set and make sure that validator which unstaked all the funds isn't present in validator set anymore
	// (execute it multiple times if needed, because it is unknown in advance how much time it is going to take until checkpoint is submitted)
	rootchainValidators := []*polybft.ValidatorInfo{}
	err = cluster.Bridge.WaitUntil(time.Second, 10*time.Second, func() (bool, error) {
		rootchainValidators, err = getRootchainValidators(l1Relayer, checkpointManagerAddr)
		if err != nil {
			return true, err
		}

		return len(rootchainValidators) == 4, nil
	})
	require.NoError(t, err)
	require.Equal(t, 4, len(rootchainValidators))

	for _, validator := range rootchainValidators {
		if validator.Address == validatorAddr {
			t.Fatalf("not expected to find validator %v in the current validator set", validator.Address)
		}
	}
}

func TestE2E_Consensus_CorrectnessOfExtraValidatorsShouldNotDependOnDelegate(t *testing.T) {
	const (
		validatorSecrets = "test-chain-1"
		delegatorSecrets = "test-chain-delegator"
		premineBalance   = "0x1B1AE4D6E2EF500000" // 500 native tokens (so that we have enough funds to fund delegator)
		epochSize        = 5
		validatorCount   = 4
		blockTime        = 2 * time.Second
	)

	cluster := framework.NewTestCluster(t, validatorCount,
		framework.WithEpochReward(100000),
		framework.WithPremineValidators(premineBalance),
		framework.WithEpochSize(epochSize))
	defer cluster.Stop()

	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(cluster.Servers[0].JSONRPCAddr()))
	require.NoError(t, err)

	// init delegator account
	_, err = cluster.InitSecrets(delegatorSecrets, 1)
	require.NoError(t, err)

	// wait for consensus to start
	require.NoError(t, cluster.WaitForBlock(2, 20*time.Second))

	// extract delegator's secrets
	delegatorSecretsPath := path.Join(cluster.Config.TmpDir, delegatorSecrets)
	delegatorAcc, err := sidechain.GetAccountFromDir(delegatorSecretsPath)
	require.NoError(t, err)

	delegatorAddr := delegatorAcc.Ecdsa.Address()

	// extract validator's secrets
	validatorSecretsPath := path.Join(cluster.Config.TmpDir, validatorSecrets)

	validatorAcc, err := sidechain.GetAccountFromDir(validatorSecretsPath)
	require.NoError(t, err)

	validatorAddr := validatorAcc.Ecdsa.Address()

	fundAmountRaw := "0xD8D726B7177A80000" // 250 native tokens
	fundAmount, err := types.ParseUint256orHex(&fundAmountRaw)
	require.NoError(t, err)

	// fund delegator
	receipt, err := txRelayer.SendTransaction(&ethgo.Transaction{
		From:  validatorAddr,
		To:    &delegatorAddr,
		Value: fundAmount,
	}, validatorAcc.Ecdsa)
	require.NoError(t, err)
	require.Equal(t, uint64(types.ReceiptSuccess), receipt.Status)

	endCh, waitCh := make(chan struct{}), make(chan struct{})

	// delegate tokens to validator in the loop to be sure that the stake of validator will be changed at end of epoch block
	go func() {
		for {
			delegationAmount := uint64(1e18)

			cluster.Servers[0].Delegate(delegationAmount, validatorSecretsPath, validatorAddr)
			require.NoError(t, err)

			select {
			case <-endCh:
				close(waitCh)

				return
			case <-time.After(blockTime / 2):
			}
		}
	}()

	require.NoError(t, cluster.WaitForBlock(6, 30*time.Second))

	close(endCh)
	<-waitCh
}
