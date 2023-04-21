package e2e

import (
	"fmt"
	"math/big"
	"path"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"

	"github.com/0xPolygon/polygon-edge/command/genesis"
	"github.com/0xPolygon/polygon-edge/command/sidechain"
	"github.com/0xPolygon/polygon-edge/consensus/polybft"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
)

func TestE2E_Consensus_Basic_WithNonValidators(t *testing.T) {
	const epochSize = 4

	cluster := framework.NewTestCluster(t, 5,
		framework.WithEpochSize(epochSize), framework.WithNonValidators(2))
	defer cluster.Stop()

	t.Run("consensus protocol", func(t *testing.T) {
		require.NoError(t, cluster.WaitForBlock(2*epochSize+1, 1*time.Minute))
	})

	t.Run("sync protocol, drop single validator node", func(t *testing.T) {
		// query the current block number, as it is a starting point for the test
		currentBlockNum, err := cluster.Servers[0].JSONRPC().Eth().BlockNumber()
		require.NoError(t, err)

		// stop one node
		node := cluster.Servers[0]
		node.Stop()

		// wait for 2 epochs to elapse, so that rest of the network progresses
		require.NoError(t, cluster.WaitForBlock(currentBlockNum+2*epochSize, 2*time.Minute))

		// start the node again
		node.Start()

		// wait 2 more epochs to elapse and make sure that stopped node managed to catch up
		require.NoError(t, cluster.WaitForBlock(currentBlockNum+4*epochSize, 2*time.Minute))
	})

	t.Run("sync protocol, drop single non-validator node", func(t *testing.T) {
		// query the current block number, as it is a starting point for the test
		currentBlockNum, err := cluster.Servers[0].JSONRPC().Eth().BlockNumber()
		require.NoError(t, err)

		// stop one non-validator node
		node := cluster.Servers[6]
		node.Stop()

		// wait for 2 epochs to elapse, so that rest of the network progresses
		require.NoError(t, cluster.WaitForBlock(currentBlockNum+2*epochSize, 2*time.Minute))

		// start the node again
		node.Start()

		// wait 2 more epochs to elapse and make sure that stopped node managed to catch up
		require.NoError(t, cluster.WaitForBlock(currentBlockNum+4*epochSize, 2*time.Minute))
	})
}

func TestE2E_Consensus_BulkDrop(t *testing.T) {
	const (
		clusterSize = 5
		bulkToDrop  = 3
		epochSize   = 5
	)

	cluster := framework.NewTestCluster(t, clusterSize,
		framework.WithEpochSize(epochSize))
	defer cluster.Stop()

	// wait for cluster to start
	cluster.WaitForReady(t)

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

	// wait to proceed to the 2nd epoch
	require.NoError(t, cluster.WaitForBlock(epochSize+1, 2*time.Minute))
}

func TestE2E_Consensus_RegisterValidator(t *testing.T) {
	const (
		validatorSize = 5
		epochSize     = 5
		epochReward   = 1000000000
	)

	var (
		firstValidatorDataDir  = fmt.Sprintf("test-chain-%d", validatorSize+1) // directory where the first validator secrets will be stored
		secondValidatorDataDir = fmt.Sprintf("test-chain-%d", validatorSize+2) // directory where the second validator secrets will be stored

		premineBalance = ethgo.Ether(2e6) // 2M native tokens (so that we have enough balance to fund new validator)
	)

	// start cluster with 'validatorSize' validators
	cluster := framework.NewTestCluster(t, validatorSize,
		framework.WithEpochSize(epochSize),
		framework.WithEpochReward(epochReward),
		framework.WithSecretsCallback(func(addresses []types.Address, config *framework.TestClusterConfig) {
			for _, a := range addresses {
				config.Premine = append(config.Premine, fmt.Sprintf("%s:%s", a, premineBalance))
			}
		}),
	)
	defer cluster.Stop()

	// first validator is the owner of ChildValidator set smart contract
	owner := cluster.Servers[0]
	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(owner.JSONRPCAddr()))
	require.NoError(t, err)

	systemState := polybft.NewSystemState(
		contracts.ValidatorSetContract,
		contracts.StateReceiverContract,
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
	cluster.InitTestServer(t, cluster.Config.ValidatorPrefix+strconv.Itoa(validatorSize+1), true, false)
	cluster.InitTestServer(t, cluster.Config.ValidatorPrefix+strconv.Itoa(validatorSize+2), true, false)

	ownerAcc, err := sidechain.GetAccountFromDir(path.Join(cluster.Config.TmpDir, ownerSecrets))
	require.NoError(t, err)

	// get the initial balance of the new validator
	initialBalance := ethgo.Ether(500000)

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
		From:                 ownerAcc.Ecdsa.Address(),
		To:                   &secondValidatorAddr,
		Value:                initialBalance,
		MaxFeePerGas:         big.NewInt(1000000000),
		MaxPriorityFeePerGas: big.NewInt(100000000),
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

	newValidatorStake := ethgo.Ether(10)

	// register the first validator with stake
	require.NoError(t, firstValidator.RegisterValidator(firstValidatorDataDir, newValidatorStake.String()))

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
		epochSize        = 5
	)

	premineBalance := ethgo.Ether(500) // 500 native tokens (so that we have enough funds to fund delegator)
	fundAmount := ethgo.Ether(250)

	cluster := framework.NewTestCluster(t, 5,
		framework.WithEpochReward(100000),
		framework.WithEpochSize(epochSize),
		framework.WithSecretsCallback(func(addresses []types.Address, config *framework.TestClusterConfig) {
			for _, a := range addresses {
				config.Premine = append(config.Premine, fmt.Sprintf("%s:%s", a, premineBalance))
				config.StakeAmounts = append(config.StakeAmounts, fmt.Sprintf("%s:%s", a, premineBalance))
			}
		}),
	)
	defer cluster.Stop()

	// init delegator account
	_, err := cluster.InitSecrets(delegatorSecrets, 1)
	require.NoError(t, err)

	srv := cluster.Servers[0]

	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(srv.JSONRPCAddr()))
	require.NoError(t, err)

	cluster.WaitForReady(t)

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
	premineAmount := ethgo.Ether(10)

	cluster := framework.NewTestCluster(t, 5,
		framework.WithBridge(),
		framework.WithEpochReward(10000),
		framework.WithEpochSize(5),
		framework.WithSecretsCallback(func(addresses []types.Address, config *framework.TestClusterConfig) {
			for _, a := range addresses {
				config.Premine = append(config.Premine, fmt.Sprintf("%s:%d", a, premineAmount))
				config.StakeAmounts = append(config.StakeAmounts, fmt.Sprintf("%s:%d", a, premineAmount))
			}
		}),
	)
	validatorSecrets := path.Join(cluster.Config.TmpDir, "test-chain-1")
	srv := cluster.Servers[0]

	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(srv.JSONRPCAddr()))
	require.NoError(t, err)

	systemState := polybft.NewSystemState(
		contracts.ValidatorSetContract,
		contracts.StateReceiverContract,
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

	polybftCfg, err := polybft.LoadPolyBFTConfig(path.Join(cluster.Config.TmpDir, chainConfigFileName))
	require.NoError(t, err)

	checkpointManagerAddr := ethgo.Address(polybftCfg.Bridge.CheckpointManagerAddr)

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

	cluster.WaitForReady(t)

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

	fundAmount := ethgo.Ether(250)

	// fund delegator
	receipt, err := txRelayer.SendTransaction(&ethgo.Transaction{
		From:  validatorAddr,
		To:    &delegatorAddr,
		Value: fundAmount,
	}, validatorAcc.Ecdsa)
	require.NoError(t, err)
	require.Equal(t, uint64(types.ReceiptSuccess), receipt.Status)

	endCh, waitCh := make(chan struct{}), make(chan struct{})

	delegationAmount := ethgo.Ether(1).Uint64()
	// delegate tokens to validator in the loop to be sure that the stake of validator will be changed at end of epoch block
	go func() {
		for {
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

func TestE2E_Consensus_MintableERC20NativeToken(t *testing.T) {
	const (
		validatorCount = 5
		epochSize      = 5
		minterPath     = "test-chain-1"

		tokenName   = "Edge Coin"
		tokenSymbol = "EDGE"
		decimals    = uint8(5)
	)

	nativeTokenAddr := ethgo.Address(contracts.NativeERC20TokenContract)

	queryNativeERC20Metadata := func(funcName string, abiType *abi.Type, relayer txrelayer.TxRelayer) interface{} {
		valueHex, err := ABICall(relayer, contractsapi.NativeERC20Mintable, nativeTokenAddr, ethgo.ZeroAddress, funcName)
		require.NoError(t, err)

		valueRaw, err := hex.DecodeHex(valueHex)
		require.NoError(t, err)

		var decodedResult map[string]interface{}

		err = abiType.DecodeStruct(valueRaw, &decodedResult)
		require.NoError(t, err)

		return decodedResult["0"]
	}

	validatorsAddrs := make([]types.Address, validatorCount)
	initialStake := ethgo.Gwei(1)
	initialBalance := int64(0)

	cluster := framework.NewTestCluster(t,
		validatorCount,
		framework.WithMintableNativeToken(true),
		framework.WithNativeTokenConfig(fmt.Sprintf("%s:%s:%d", tokenName, tokenSymbol, decimals)),
		framework.WithEpochSize(epochSize),
		framework.WithSecretsCallback(func(addrs []types.Address, config *framework.TestClusterConfig) {
			for i, addr := range addrs {
				// first one is the owner of the NativeERC20Mintable SC
				// and it should have premine set to default 1M tokens
				// (it is going to send mint transactions to all the other validators)
				if i > 0 {
					// premine other validators with as minimum stake as possible just to ensure liveness of consensus protocol
					config.Premine = append(config.Premine, fmt.Sprintf("%s:%d", addr, initialBalance))
					config.StakeAmounts = append(config.StakeAmounts, fmt.Sprintf("%s:%d", addr, initialStake))
				}
				validatorsAddrs[i] = addr
			}
		}))
	defer cluster.Stop()

	minterSrv := cluster.Servers[0]
	targetJSONRPC := minterSrv.JSONRPC()

	minterAcc, err := sidechain.GetAccountFromDir(minterSrv.DataDir())
	require.NoError(t, err)

	cluster.WaitForReady(t)

	// initialize tx relayer
	relayer, err := txrelayer.NewTxRelayer(txrelayer.WithClient(targetJSONRPC))
	require.NoError(t, err)

	// check are native token metadata correctly initialized
	stringABIType := abi.MustNewType("tuple(string)")
	uint8ABIType := abi.MustNewType("tuple(uint8)")

	name := queryNativeERC20Metadata("name", stringABIType, relayer)
	require.Equal(t, tokenName, name)

	symbol := queryNativeERC20Metadata("symbol", stringABIType, relayer)
	require.Equal(t, tokenSymbol, symbol)

	decimalsCount := queryNativeERC20Metadata("decimals", uint8ABIType, relayer)
	require.Equal(t, decimals, decimalsCount)

	// send mint transactions
	mintFn, exists := contractsapi.NativeERC20Mintable.Abi.Methods["mint"]
	require.True(t, exists)

	mintAmount := ethgo.Ether(10)

	// make sure minter account can mint tokens
	// (first account, which is minter sends mint transactions to all the other validators)
	for _, addr := range validatorsAddrs[1:] {
		balance, err := targetJSONRPC.Eth().GetBalance(ethgo.Address(addr), ethgo.Latest)
		require.NoError(t, err)
		t.Logf("Pre-mint balance: %v=%d\n", addr, balance)

		mintInput, err := mintFn.Encode([]interface{}{addr, mintAmount})
		require.NoError(t, err)

		receipt, err := relayer.SendTransaction(
			&ethgo.Transaction{
				To:    &nativeTokenAddr,
				Input: mintInput,
			}, minterAcc.Ecdsa)
		require.NoError(t, err)
		require.Equal(t, uint64(types.ReceiptSuccess), receipt.Status)

		balance, err = targetJSONRPC.Eth().GetBalance(ethgo.Address(addr), ethgo.Latest)
		require.NoError(t, err)

		t.Logf("Post-mint balance: %v=%d\n", addr, balance)
		require.Equal(t, new(big.Int).Add(mintAmount, big.NewInt(initialBalance)), balance)
	}

	minterBalance, err := targetJSONRPC.Eth().GetBalance(minterAcc.Ecdsa.Address(), ethgo.Latest)
	require.NoError(t, err)
	require.Equal(t, ethgo.Ether(1e6), minterBalance)

	// try sending mint transaction from non minter account and make sure it would fail
	nonMinterAcc, err := sidechain.GetAccountFromDir(cluster.Servers[1].DataDir())
	require.NoError(t, err)

	mintInput, err := mintFn.Encode([]interface{}{validatorsAddrs[2], ethgo.Ether(1)})
	require.NoError(t, err)

	receipt, err := relayer.SendTransaction(
		&ethgo.Transaction{
			To:    &nativeTokenAddr,
			Input: mintInput,
		}, nonMinterAcc.Ecdsa)
	require.NoError(t, err)
	require.Equal(t, uint64(types.ReceiptFailed), receipt.Status)
}
