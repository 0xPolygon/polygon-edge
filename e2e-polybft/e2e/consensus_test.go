package e2e

import (
	"fmt"
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
		validatorSize  = 5
		epochSize      = 5
		epochReward    = 1000000000
		premineBalance = "0x1A784379D99DB42000000" // 2M native tokens (so that we have enough balance to fund new validator)
	)

	var (
		firstValidatorDataDir   = fmt.Sprintf("test-chain-%d", validatorSize+1) // directory where the first validator secrets will be stored
		secondValidatorDataDir  = fmt.Sprintf("test-chain-%d", validatorSize+2) // directory where the second validator secrets will be stored
		newValidatorInitBalance = "500000000000000000000000"                    // 500k - balance which will be transferred to the new validator
		newValidatorStakeRaw    = "0x8AC7230489E80000"                          // 10 native tokens  - amount which will be staked by the new validator
	)

	newValidatorStake, err := types.ParseUint256orHex(&newValidatorStakeRaw)
	require.NoError(t, err)

	// start cluster with 'validatorSize' validators
	cluster := framework.NewTestCluster(t, validatorSize,
		framework.WithEpochSize(epochSize),
		framework.WithEpochReward(epochReward),
		framework.WithSecretsCallback(func(addresses []types.Address, config *framework.TestClusterConfig) {
			for _, a := range addresses {
				config.PremineValidators = append(config.PremineValidators, fmt.Sprintf("%s:%s", a, premineBalance))
			}
		}),
	)
	defer cluster.Stop()

	// first validator is the owner of ChildValidator set smart contract
	owner := cluster.Servers[0]
	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(owner.JSONRPCAddr()))
	require.NoError(t, err)

	systemState := polybft.NewSystemState(
		&polybft.PolyBFTConfig{
			StateReceiverAddr: contracts.StateReceiverContract,
			ValidatorSetAddr:  contracts.ValidatorSetContract},
		&e2eStateProvider{txRelayer: txRelayer})

	// create the first account and extract the address
	addrs, err := cluster.InitSecrets(firstValidatorDataDir, 1)
	require.NoError(t, err)

	firstValidatorAddr := ethgo.Address(addrs[0])

	// create the second account and extract the address
	addrs, err = cluster.InitSecrets(secondValidatorDataDir, 1)
	require.NoError(t, err)

	secondValidatorAddr := ethgo.Address(addrs[0])

	// assert that accounts are created
	validatorSecrets, err := genesis.GetValidatorKeyFiles(cluster.Config.TmpDir, cluster.Config.ValidatorPrefix)
	require.NoError(t, err)
	require.Equal(t, validatorSize+2, len(validatorSecrets))

	// collect owners validator secrets
	ownerSecrets := validatorSecrets[0]

	// wait for consensus to start
	require.NoError(t, cluster.WaitForBlock(1, 10*time.Second))

	genesisBlock, err := owner.JSONRPC().Eth().GetBlockByNumber(0, false)
	require.NoError(t, err)

	extra, err := polybft.GetIbftExtra(genesisBlock.ExtraData)
	require.NoError(t, err)

	// on genesis block all validators are marked as added, which makes initial validator set
	initialValidators := extra.Validators.Added

	// owner whitelists both new validators
	require.NoError(t, owner.WhitelistValidator(firstValidatorAddr.String(), ownerSecrets))
	require.NoError(t, owner.WhitelistValidator(secondValidatorAddr.String(), ownerSecrets))

	// start the first and the second validator
	cluster.InitTestServer(t, validatorSize+1, true, false)
	cluster.InitTestServer(t, validatorSize+2, true, false)

	ownerAcc, err := sidechain.GetAccountFromDir(path.Join(cluster.Config.TmpDir, ownerSecrets))
	require.NoError(t, err)

	// get the initial balance of the new validator
	initialBalance, ok := new(big.Int).SetString(newValidatorInitBalance, 10)
	require.True(t, ok)

	// send some tokens from the owner to the first validator so that the first validator can register and stake
	receipt, err := txRelayer.SendTransaction(&ethgo.Transaction{
		From:  ownerAcc.Ecdsa.Address(),
		To:    &firstValidatorAddr,
		Value: initialBalance,
	}, ownerAcc.Ecdsa)
	require.NoError(t, err)
	require.Equal(t, uint64(types.ReceiptSuccess), receipt.Status)

	// send some tokens from the owner to the second validator so that the second validator can register and stake
	receipt, err = txRelayer.SendTransaction(&ethgo.Transaction{
		From:  ownerAcc.Ecdsa.Address(),
		To:    &secondValidatorAddr,
		Value: initialBalance,
	}, ownerAcc.Ecdsa)
	require.NoError(t, err)
	require.Equal(t, uint64(types.ReceiptSuccess), receipt.Status)

	// collect the first and the second validator from the cluster
	firstValidator := cluster.Servers[validatorSize]
	secondValidator := cluster.Servers[validatorSize+1]

	// wait for the first validator's balance to be received
	firstBalance, err := firstValidator.WaitForNonZeroBalance(firstValidatorAddr, 5*time.Second)
	require.NoError(t, err)
	t.Logf("First validator balance=%d\n", firstBalance)

	// wait for the first validator's balance to be received
	secondBalance, err := secondValidator.WaitForNonZeroBalance(secondValidatorAddr, 5*time.Second)
	require.NoError(t, err)
	t.Logf("Second validator balance=%d\n", secondBalance)

	// register the first validator with stake
	require.NoError(t, firstValidator.RegisterValidator(firstValidatorDataDir, newValidatorStakeRaw))

	// register the second validator without stake
	require.NoError(t, secondValidator.RegisterValidator(secondValidatorDataDir, ""))

	// stake manually for the second validator
	require.NoError(t, secondValidator.Stake(newValidatorStake.Uint64()))

	validators := polybft.AccountSet{}
	// assert that new validator is among validator set
	require.NoError(t, cluster.WaitUntil(20*time.Second, 1*time.Second, func() bool {
		// query validators
		validators, err = systemState.GetValidatorSet()
		require.NoError(t, err)

		return validators.ContainsAddress((types.Address(firstValidatorAddr))) &&
			validators.ContainsAddress((types.Address(secondValidatorAddr)))
	}))

	// assert that next validators hash is correct
	block, err := owner.JSONRPC().Eth().GetBlockByNumber(ethgo.Latest, false)
	require.NoError(t, err)
	t.Logf("Block Number=%d\n", block.Number)

	// apply all deltas from epoch ending blocks
	nextValidators := initialValidators

	// apply all the deltas to the initial validator set
	latestEpochEndBlock := block.Number - (block.Number % epochSize)
	for blockNum := uint64(epochSize); blockNum <= latestEpochEndBlock; blockNum += epochSize {
		block, err = owner.JSONRPC().Eth().GetBlockByNumber(ethgo.BlockNumber(blockNum), false)
		require.NoError(t, err)

		extra, err = polybft.GetIbftExtra(block.ExtraData)
		require.NoError(t, err)
		require.NotNil(t, extra.Checkpoint)

		t.Logf("Block Number: %d, Delta: %v\n", blockNum, extra.Validators)

		nextValidators, err = nextValidators.ApplyDelta(extra.Validators)
		require.NoError(t, err)
	}

	// assert that correct validators hash gets submitted
	nextValidatorsHash, err := nextValidators.Hash()
	require.NoError(t, err)
	require.Equal(t, extra.Checkpoint.NextValidatorsHash, nextValidatorsHash)

	// query the first validator
	firstValidatorInfo, err := sidechain.GetValidatorInfo(firstValidatorAddr, txRelayer)
	require.NoError(t, err)

	// assert the first validator's stake
	t.Logf("First validator stake=%d\n", firstValidatorInfo.TotalStake)
	require.Equal(t, newValidatorStake, firstValidatorInfo.TotalStake)

	// query the second validatorr
	secondValidatorInfo, err := sidechain.GetValidatorInfo(secondValidatorAddr, txRelayer)
	require.NoError(t, err)

	// assert the second validator's stake
	t.Logf("Second validator stake=%d\n", secondValidatorInfo.TotalStake)
	require.Equal(t, newValidatorStake, secondValidatorInfo.TotalStake)

	// wait 3 more epochs, so that rewards get accumulated to the registered validator account
	currentBlock, err := owner.JSONRPC().Eth().BlockNumber()
	require.NoError(t, err)
	require.NoError(t, cluster.WaitForBlock(currentBlock+3*epochSize, 2*time.Minute))

	// query the first validator info again
	firstValidatorInfo, err = sidechain.GetValidatorInfo(firstValidatorAddr, txRelayer)
	require.NoError(t, err)

	// check if the first validator has signed any prposals
	firstSealed, err := firstValidator.HasValidatorSealed(currentBlock, currentBlock+3*epochSize, nextValidators, firstValidatorAddr)
	require.NoError(t, err)

	if firstSealed {
		// assert registered validator's rewards)
		t.Logf("First validator rewards (block %d)=%d\n", currentBlock, firstValidatorInfo.WithdrawableRewards)
		require.True(t, firstValidatorInfo.WithdrawableRewards.Cmp(big.NewInt(0)) > 0)
	}

	// query the second validator info again
	secondValidatorInfo, err = sidechain.GetValidatorInfo(secondValidatorAddr, txRelayer)
	require.NoError(t, err)

	// check if the second validator has signed any prposals
	secondSealed, err := secondValidator.HasValidatorSealed(currentBlock, currentBlock+3*epochSize, nextValidators, secondValidatorAddr)
	require.NoError(t, err)

	if secondSealed {
		// assert registered validator's rewards
		t.Logf("Second validator rewards (block %d)=%d\n", currentBlock, secondValidatorInfo.WithdrawableRewards)
		require.True(t, secondValidatorInfo.WithdrawableRewards.Cmp(big.NewInt(0)) > 0)
	}
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
		framework.WithEpochSize(epochSize),
		framework.WithSecretsCallback(func(addresses []types.Address, config *framework.TestClusterConfig) {
			for _, a := range addresses {
				config.PremineValidators = append(config.PremineValidators, fmt.Sprintf("%s:%s", a, premineBalance))
			}
		}),
	)
	defer cluster.Stop()

	// init delegator account
	_, err = cluster.InitSecrets(delegatorSecrets, 1)
	require.NoError(t, err)

	srv := cluster.Servers[0]

	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(srv.JSONRPCAddr()))
	require.NoError(t, err)

	// wait for consensus to start
	require.NoError(t, cluster.WaitForBlock(1, 10*time.Second))

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
	require.NoError(t, cluster.WaitForBlock(currentBlockNum+2*epochSize, 1*time.Minute))

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
	require.NoError(t, cluster.WaitForBlock(currentBlockNum+epochSize, time.Minute))

	// assert that delegator doesn't receive any rewards
	delegatorBalance, delegatorReward = getDelegatorInfo()
	require.True(t, delegatorReward.Cmp(big.NewInt(0)) == 0)
	require.True(t, balanceBeforeWithdraw.Cmp(delegatorBalance) == -1)
}

func TestE2E_Consensus_Validator_Unstake(t *testing.T) {
	const premineAmount = "10000000000000000000" // 10 native tokens

	cluster := framework.NewTestCluster(t, 5,
		framework.WithBridge(),
		framework.WithEpochReward(10000),
		framework.WithEpochSize(5),
		framework.WithSecretsCallback(func(addresses []types.Address, config *framework.TestClusterConfig) {
			for _, a := range addresses {
				config.PremineValidators = append(config.PremineValidators, fmt.Sprintf("%s:%s", a, premineAmount))
			}
		}),
	)
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

			err = cluster.Servers[0].Delegate(delegationAmount, validatorSecretsPath, validatorAddr)
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
