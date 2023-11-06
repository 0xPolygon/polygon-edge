package e2e

import (
	"fmt"
	"math/big"
	"path"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
	"github.com/umbracle/ethgo/wallet"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/genesis"
	"github.com/0xPolygon/polygon-edge/command/sidechain"
	"github.com/0xPolygon/polygon-edge/consensus/polybft"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
)

var uint256ABIType = abi.MustNewType("tuple(uint256)")

func TestE2E_Consensus_Basic_WithNonValidators(t *testing.T) {
	const (
		epochSize     = 4
		validatorsNum = 5
	)

	cluster := framework.NewTestCluster(t, validatorsNum,
		framework.WithEpochSize(epochSize),
		framework.WithNonValidators(2),
		framework.WithTestRewardToken(),
	)
	defer cluster.Stop()

	cluster.WaitForReady(t)

	// initialize tx relayer
	relayer, err := txrelayer.NewTxRelayer(txrelayer.WithClient(cluster.Servers[0].JSONRPC()))
	require.NoError(t, err)

	// because we are pre-mining native tokens to validators
	initialTotalSupply := new(big.Int).Mul(big.NewInt(validatorsNum), command.DefaultPremineBalance)

	// check if initial total supply of native ERC20 token is the same as expected
	totalSupply := queryNativeERC20Metadata(t, "totalSupply", uint256ABIType, relayer)
	require.True(t, initialTotalSupply.Cmp(totalSupply.(*big.Int)) == 0) //nolint:forcetypeassert

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
		bulkToDrop  = 4
		epochSize   = 5
	)

	cluster := framework.NewTestCluster(t, clusterSize,
		framework.WithEpochSize(epochSize),
		framework.WithBlockTime(time.Second),
		framework.WithTestRewardToken(),
	)
	defer cluster.Stop()

	// wait for cluster to start
	cluster.WaitForReady(t)

	var wg sync.WaitGroup
	// drop bulk of nodes from cluster
	for i := 0; i < bulkToDrop; i++ {
		node := cluster.Servers[i]

		wg.Add(1)

		go func(node *framework.TestServer) {
			defer wg.Done()
			node.Stop()
		}(node)
	}

	wg.Wait()

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
		validatorSetSize = 5
		epochSize        = 5
	)

	var (
		firstValidatorDataDir  = fmt.Sprintf("test-chain-%d", validatorSetSize+1) // directory where the first validator secrets will be stored
		secondValidatorDataDir = fmt.Sprintf("test-chain-%d", validatorSetSize+2) // directory where the second validator secrets will be stored

		initMinterBalance = ethgo.Ether(4e6)
		premineBalance    = ethgo.Ether(2e6) // 2M native tokens (so that we have enough balance to fund new validator)
	)

	minter, err := wallet.GenerateKey()
	require.NoError(t, err)

	// start cluster with 'validatorSize' validators
	cluster := framework.NewTestCluster(t, validatorSetSize,
		framework.WithEpochSize(epochSize),
		framework.WithEpochReward(int(ethgo.Ether(1).Uint64())),
		framework.WithNativeTokenConfig(fmt.Sprintf(framework.NativeTokenMintableTestCfg, minter.Address())),
		framework.WithSecretsCallback(func(addresses []types.Address, config *framework.TestClusterConfig) {
			config.Premine = append(config.Premine, fmt.Sprintf("%s:%s", minter.Address(), initMinterBalance))
			for _, a := range addresses {
				config.Premine = append(config.Premine, fmt.Sprintf("%s:%s", a, premineBalance))
			}
		}),
	)
	defer cluster.Stop()

	cluster.WaitForReady(t)

	// first validator is the owner of ChildValidator set smart contract
	owner := cluster.Servers[0]
	childChainRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(owner.JSONRPCAddr()))
	require.NoError(t, err)

	rootChainRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(cluster.Bridge.JSONRPCAddr()))
	require.NoError(t, err)

	polybftConfig, err := polybft.LoadPolyBFTConfig(path.Join(cluster.Config.TmpDir, chainConfigFileName))
	require.NoError(t, err)

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
	require.Equal(t, validatorSetSize+2, len(validatorSecrets))

	// collect owners validator secrets
	firstValidatorSecrets := validatorSecrets[validatorSetSize]
	secondValidatorSecrets := validatorSecrets[validatorSetSize+1]

	genesisBlock, err := owner.JSONRPC().Eth().GetBlockByNumber(0, false)
	require.NoError(t, err)

	_, err = polybft.GetIbftExtra(genesisBlock.ExtraData)
	require.NoError(t, err)

	// owner whitelists both new validators
	require.NoError(t, owner.WhitelistValidators([]string{
		firstValidatorAddr.String(),
		secondValidatorAddr.String(),
	}, polybftConfig.Bridge.CustomSupernetManagerAddr))

	// set the initial balance of the new validators at the rootchain
	initialBalance := ethgo.Ether(500)

	// fund first new validator
	err = cluster.Bridge.FundValidators(polybftConfig.Bridge.StakeTokenAddr,
		[]string{path.Join(cluster.Config.TmpDir, firstValidatorSecrets)}, []*big.Int{initialBalance})
	require.NoError(t, err)

	// fund second new validator
	err = cluster.Bridge.FundValidators(polybftConfig.Bridge.StakeTokenAddr,
		[]string{path.Join(cluster.Config.TmpDir, secondValidatorSecrets)}, []*big.Int{initialBalance})
	require.NoError(t, err)

	// first validator's balance to be received
	firstBalance, err := rootChainRelayer.Client().Eth().GetBalance(firstValidatorAddr, ethgo.Latest)
	require.NoError(t, err)
	t.Logf("First validator balance=%d\n", firstBalance)

	// second validator's balance to be received
	secondBalance, err := rootChainRelayer.Client().Eth().GetBalance(secondValidatorAddr, ethgo.Latest)
	require.NoError(t, err)
	t.Logf("Second validator balance=%d\n", secondBalance)

	// start the first and the second validator
	cluster.InitTestServer(t, cluster.Config.ValidatorPrefix+strconv.Itoa(validatorSetSize+1),
		cluster.Bridge.JSONRPCAddr(), framework.Validator)

	cluster.InitTestServer(t, cluster.Config.ValidatorPrefix+strconv.Itoa(validatorSetSize+2),
		cluster.Bridge.JSONRPCAddr(), framework.Validator)

	// collect the first and the second validator from the cluster
	firstValidator := cluster.Servers[validatorSetSize]
	secondValidator := cluster.Servers[validatorSetSize+1]

	initialStake := ethgo.Ether(499)

	// register the first validator with stake
	require.NoError(t, firstValidator.RegisterValidator(polybftConfig.Bridge.CustomSupernetManagerAddr))

	// register the second validator without stake
	require.NoError(t, secondValidator.RegisterValidator(polybftConfig.Bridge.CustomSupernetManagerAddr))

	// stake manually for the first validator
	require.NoError(t, firstValidator.Stake(polybftConfig, initialStake))

	// stake manually for the second validator
	require.NoError(t, secondValidator.Stake(polybftConfig, initialStake))

	firstValidatorInfo, err := sidechain.GetValidatorInfo(firstValidatorAddr,
		polybftConfig.Bridge.CustomSupernetManagerAddr, polybftConfig.Bridge.StakeManagerAddr,
		polybftConfig.SupernetID, rootChainRelayer, childChainRelayer)
	require.NoError(t, err)
	require.True(t, firstValidatorInfo.IsActive)
	require.True(t, firstValidatorInfo.Stake.Cmp(initialStake) == 0)

	secondValidatorInfo, err := sidechain.GetValidatorInfo(secondValidatorAddr,
		polybftConfig.Bridge.CustomSupernetManagerAddr, polybftConfig.Bridge.StakeManagerAddr,
		polybftConfig.SupernetID, rootChainRelayer, childChainRelayer)
	require.NoError(t, err)
	require.True(t, secondValidatorInfo.IsActive)
	require.True(t, secondValidatorInfo.Stake.Cmp(initialStake) == 0)

	// wait for the stake to be bridged
	require.NoError(t, cluster.WaitForBlock(polybftConfig.EpochSize*4, time.Minute))

	checkpointManagerAddr := ethgo.Address(polybftConfig.Bridge.CheckpointManagerAddr)

	// check if the validators are added to active validator set
	rootchainValidators := []*polybft.ValidatorInfo{}
	err = cluster.Bridge.WaitUntil(time.Second, time.Minute, func() (bool, error) {
		rootchainValidators, err = getCheckpointManagerValidators(rootChainRelayer, checkpointManagerAddr)
		if err != nil {
			return true, err
		}

		return len(rootchainValidators) == validatorSetSize+2, nil
	})

	require.NoError(t, err)
	require.Equal(t, validatorSetSize+2, len(rootchainValidators))

	var (
		isFirstValidatorFound  bool
		isSecondValidatorFound bool
	)

	for _, v := range rootchainValidators {
		if v.Address == firstValidatorAddr {
			isFirstValidatorFound = true
		} else if v.Address == secondValidatorAddr {
			isSecondValidatorFound = true
		}
	}

	// new validators should be in the active validator set
	require.True(t, isFirstValidatorFound)
	require.True(t, isSecondValidatorFound)

	currentBlock, err := owner.JSONRPC().Eth().GetBlockByNumber(ethgo.Latest, false)
	require.NoError(t, err)

	// wait for couple of epochs to have some rewards accumulated
	require.NoError(t, cluster.WaitForBlock(currentBlock.Number+(polybftConfig.EpochSize*2), time.Minute))

	bigZero := big.NewInt(0)

	firstValidatorInfo, err = sidechain.GetValidatorInfo(firstValidatorAddr,
		polybftConfig.Bridge.CustomSupernetManagerAddr, polybftConfig.Bridge.StakeManagerAddr,
		polybftConfig.SupernetID, rootChainRelayer, childChainRelayer)
	require.NoError(t, err)
	require.True(t, firstValidatorInfo.IsActive)
	require.True(t, firstValidatorInfo.WithdrawableRewards.Cmp(bigZero) > 0)

	secondValidatorInfo, err = sidechain.GetValidatorInfo(secondValidatorAddr,
		polybftConfig.Bridge.CustomSupernetManagerAddr, polybftConfig.Bridge.StakeManagerAddr,
		polybftConfig.SupernetID, rootChainRelayer, childChainRelayer)
	require.NoError(t, err)
	require.True(t, secondValidatorInfo.IsActive)
	require.True(t, secondValidatorInfo.WithdrawableRewards.Cmp(bigZero) > 0)
}

func TestE2E_Consensus_Validator_Unstake(t *testing.T) {
	var (
		premineAmount = ethgo.Ether(10)
		minterBalance = ethgo.Ether(1e6)
	)

	minter, err := wallet.GenerateKey()
	require.NoError(t, err)

	cluster := framework.NewTestCluster(t, 5,
		framework.WithEpochReward(int(ethgo.Ether(1).Uint64())),
		framework.WithEpochSize(5),
		framework.WithNativeTokenConfig(fmt.Sprintf(framework.NativeTokenMintableTestCfg, minter.Address())),
		framework.WithSecretsCallback(func(addresses []types.Address, config *framework.TestClusterConfig) {
			config.Premine = append(config.Premine, fmt.Sprintf("%s:%d", minter.Address(), minterBalance))
			for _, a := range addresses {
				config.Premine = append(config.Premine, fmt.Sprintf("%s:%d", a, premineAmount))
				config.StakeAmounts = append(config.StakeAmounts, new(big.Int).Set(premineAmount))
			}
		}),
	)

	polybftCfg, err := polybft.LoadPolyBFTConfig(path.Join(cluster.Config.TmpDir, chainConfigFileName))
	require.NoError(t, err)

	srv := cluster.Servers[0]

	childChainRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(srv.JSONRPCAddr()))
	require.NoError(t, err)

	rootChainRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(cluster.Bridge.JSONRPCAddr()))
	require.NoError(t, err)

	validatorAcc, err := sidechain.GetAccountFromDir(srv.DataDir())
	require.NoError(t, err)

	cluster.WaitForReady(t)

	initialValidatorBalance, err := srv.JSONRPC().Eth().GetBalance(validatorAcc.Ecdsa.Address(), ethgo.Latest)
	require.NoError(t, err)
	t.Logf("Balance (before unstake)=%d\n", initialValidatorBalance)

	validatorAddr := validatorAcc.Ecdsa.Address()

	// wait for some rewards to get accumulated
	require.NoError(t, cluster.WaitForBlock(polybftCfg.EpochSize*3, time.Minute))

	validatorInfo, err := sidechain.GetValidatorInfo(validatorAddr,
		polybftCfg.Bridge.CustomSupernetManagerAddr, polybftCfg.Bridge.StakeManagerAddr,
		polybftCfg.SupernetID, rootChainRelayer, childChainRelayer)
	require.NoError(t, err)
	require.True(t, validatorInfo.IsActive)

	initialStake := validatorInfo.Stake
	t.Logf("Stake (before unstake)=%d\n", initialStake)

	reward := validatorInfo.WithdrawableRewards
	t.Logf("Rewards=%d\n", reward)
	require.Greater(t, reward.Uint64(), uint64(0))

	// unstake entire balance (which should remove validator from the validator set in next epoch)
	require.NoError(t, srv.Unstake(initialStake))

	// wait for one epoch to withdraw from child
	require.NoError(t, cluster.WaitForBlock(polybftCfg.EpochSize*4, time.Minute))

	// withdraw from child
	require.NoError(t, srv.WithdrawChildChain())

	currentBlock, err := srv.JSONRPC().Eth().GetBlockByNumber(ethgo.Latest, false)
	require.NoError(t, err)

	currentExtra, err := polybft.GetIbftExtra(currentBlock.ExtraData)
	require.NoError(t, err)

	t.Logf("Latest block number: %d, epoch number: %d\n", currentBlock.Number, currentExtra.Checkpoint.EpochNumber)

	currentEpoch := currentExtra.Checkpoint.EpochNumber

	// wait for checkpoint to be submitted
	require.NoError(t, waitForRootchainEpoch(currentEpoch, time.Minute,
		rootChainRelayer, polybftCfg.Bridge.CheckpointManagerAddr))

	exitEventID := uint64(1)

	// send exit transaction to exit helper
	err = cluster.Bridge.SendExitTransaction(polybftCfg.Bridge.ExitHelperAddr, exitEventID, srv.JSONRPCAddr())
	require.NoError(t, err)

	// check that validator is no longer active (out of validator set)
	validatorInfo, err = sidechain.GetValidatorInfo(validatorAddr,
		polybftCfg.Bridge.CustomSupernetManagerAddr, polybftCfg.Bridge.StakeManagerAddr,
		polybftCfg.SupernetID, rootChainRelayer, childChainRelayer)
	require.NoError(t, err)
	require.False(t, validatorInfo.IsActive)
	require.True(t, validatorInfo.Stake.Cmp(big.NewInt(0)) == 0)

	t.Logf("Stake (after unstake)=%d\n", validatorInfo.Stake)

	balanceBeforeRewardsWithdraw, err := srv.JSONRPC().Eth().GetBalance(validatorAcc.Ecdsa.Address(), ethgo.Latest)
	require.NoError(t, err)
	t.Logf("Balance (before withdraw rewards)=%d\n", balanceBeforeRewardsWithdraw)

	// withdraw pending rewards
	require.NoError(t, srv.WithdrawRewards())

	newValidatorBalance, err := srv.JSONRPC().Eth().GetBalance(validatorAcc.Ecdsa.Address(), ethgo.Latest)
	require.NoError(t, err)
	t.Logf("Balance (after withdrawal of rewards)=%s\n", newValidatorBalance)
	require.True(t, newValidatorBalance.Cmp(balanceBeforeRewardsWithdraw) > 0)

	l1Relayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(cluster.Bridge.JSONRPCAddr()))
	require.NoError(t, err)

	checkpointManagerAddr := ethgo.Address(polybftCfg.Bridge.CheckpointManagerAddr)

	// query rootchain validator set and make sure that validator which unstaked all the funds isn't present in validator set anymore
	// (execute it multiple times if needed, because it is unknown in advance how much time it is going to take until checkpoint is submitted)
	rootchainValidators := []*polybft.ValidatorInfo{}
	err = cluster.Bridge.WaitUntil(time.Second, 10*time.Second, func() (bool, error) {
		rootchainValidators, err = getCheckpointManagerValidators(l1Relayer, checkpointManagerAddr)
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

func TestE2E_Consensus_MintableERC20NativeToken(t *testing.T) {
	const (
		validatorCount = 5
		epochSize      = 5

		tokenName   = "Edge Coin"
		tokenSymbol = "EDGE"
		decimals    = uint8(5)
	)

	validatorsAddrs := make([]types.Address, validatorCount)
	initValidatorsBalance := ethgo.Ether(1)
	initMinterBalance := ethgo.Ether(100000)

	minter, err := wallet.GenerateKey()
	require.NoError(t, err)

	// because we are using native token as reward wallet, and it has default premine balance
	initialTotalSupply := new(big.Int).Set(command.DefaultPremineBalance)

	cluster := framework.NewTestCluster(t,
		validatorCount,
		framework.WithNativeTokenConfig(
			fmt.Sprintf("%s:%s:%d:true:%s", tokenName, tokenSymbol, decimals, minter.Address())),
		framework.WithEpochSize(epochSize),
		framework.WithSecretsCallback(func(addrs []types.Address, config *framework.TestClusterConfig) {
			config.Premine = append(config.Premine, fmt.Sprintf("%s:%d", minter.Address(), initMinterBalance))
			initialTotalSupply.Add(initialTotalSupply, initMinterBalance)

			for i, addr := range addrs {
				config.Premine = append(config.Premine, fmt.Sprintf("%s:%d", addr, initValidatorsBalance))
				config.StakeAmounts = append(config.StakeAmounts, new(big.Int).Set(initValidatorsBalance))
				validatorsAddrs[i] = addr
				initialTotalSupply.Add(initialTotalSupply, initValidatorsBalance)
			}
		}))
	defer cluster.Stop()

	targetJSONRPC := cluster.Servers[0].JSONRPC()

	cluster.WaitForReady(t)

	// initialize tx relayer
	relayer, err := txrelayer.NewTxRelayer(txrelayer.WithClient(targetJSONRPC))
	require.NoError(t, err)

	// check are native token metadata correctly initialized
	stringABIType := abi.MustNewType("tuple(string)")
	uint8ABIType := abi.MustNewType("tuple(uint8)")

	totalSupply := queryNativeERC20Metadata(t, "totalSupply", uint256ABIType, relayer)
	require.True(t, initialTotalSupply.Cmp(totalSupply.(*big.Int)) == 0) //nolint:forcetypeassert

	// check if initial total supply of native ERC20 token is the same as expected
	name := queryNativeERC20Metadata(t, "name", stringABIType, relayer)
	require.Equal(t, tokenName, name)

	symbol := queryNativeERC20Metadata(t, "symbol", stringABIType, relayer)
	require.Equal(t, tokenSymbol, symbol)

	decimalsCount := queryNativeERC20Metadata(t, "decimals", uint8ABIType, relayer)
	require.Equal(t, decimals, decimalsCount)

	// send mint transactions
	mintFn, exists := contractsapi.NativeERC20Mintable.Abi.Methods["mint"]
	require.True(t, exists)

	mintAmount := ethgo.Ether(10)
	nativeTokenAddr := ethgo.Address(contracts.NativeERC20TokenContract)

	// make sure minter account can mint tokens
	for _, addr := range validatorsAddrs {
		balance, err := targetJSONRPC.Eth().GetBalance(ethgo.Address(addr), ethgo.Latest)
		require.NoError(t, err)
		t.Logf("Pre-mint balance: %v=%d\n", addr, balance)

		mintInput, err := mintFn.Encode([]interface{}{addr, mintAmount})
		require.NoError(t, err)

		receipt, err := relayer.SendTransaction(
			&ethgo.Transaction{
				To:    &nativeTokenAddr,
				Input: mintInput,
				Type:  ethgo.TransactionDynamicFee,
			}, minter)
		require.NoError(t, err)
		require.Equal(t, uint64(types.ReceiptSuccess), receipt.Status)

		balance, err = targetJSONRPC.Eth().GetBalance(ethgo.Address(addr), ethgo.Latest)
		require.NoError(t, err)

		t.Logf("Post-mint balance: %v=%d\n", addr, balance)
		require.Equal(t, new(big.Int).Add(initValidatorsBalance, mintAmount), balance)
	}

	// assert that minter balance remained the same
	minterBalance, err := targetJSONRPC.Eth().GetBalance(minter.Address(), ethgo.Latest)
	require.NoError(t, err)
	require.Equal(t, initMinterBalance, minterBalance)

	// try sending mint transaction from non minter account and make sure it would fail
	nonMinterAcc, err := sidechain.GetAccountFromDir(cluster.Servers[1].DataDir())
	require.NoError(t, err)

	mintInput, err := mintFn.Encode([]interface{}{validatorsAddrs[0], ethgo.Ether(1)})
	require.NoError(t, err)

	receipt, err := relayer.SendTransaction(
		&ethgo.Transaction{
			To:    &nativeTokenAddr,
			Input: mintInput,
			Type:  ethgo.TransactionDynamicFee,
		}, nonMinterAcc.Ecdsa)
	require.Error(t, err)
	require.Nil(t, receipt)
}

func TestE2E_Consensus_CustomRewardToken(t *testing.T) {
	const epochSize = 5

	cluster := framework.NewTestCluster(t, 5,
		framework.WithEpochSize(epochSize),
		framework.WithEpochReward(1000000),
		framework.WithTestRewardToken(),
	)
	defer cluster.Stop()

	cluster.WaitForReady(t)

	// wait for couple of epochs to accumulate some rewards
	require.NoError(t, cluster.WaitForBlock(epochSize*3, 3*time.Minute))

	// first validator is the owner of ChildValidator set smart contract
	owner := cluster.Servers[0]
	childChainRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(owner.JSONRPCAddr()))
	require.NoError(t, err)

	rootChainRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(cluster.Bridge.JSONRPCAddr()))
	require.NoError(t, err)

	polybftConfig, err := polybft.LoadPolyBFTConfig(path.Join(cluster.Config.TmpDir, chainConfigFileName))
	require.NoError(t, err)

	validatorAcc, err := sidechain.GetAccountFromDir(owner.DataDir())
	require.NoError(t, err)

	validatorInfo, err := sidechain.GetValidatorInfo(validatorAcc.Ecdsa.Address(),
		polybftConfig.Bridge.CustomSupernetManagerAddr, polybftConfig.Bridge.StakeManagerAddr,
		polybftConfig.SupernetID, rootChainRelayer, childChainRelayer)
	t.Logf("[Validator#%v] Witdhrawable rewards=%d\n", validatorInfo.Address, validatorInfo.WithdrawableRewards)

	require.NoError(t, err)
	require.True(t, validatorInfo.WithdrawableRewards.Cmp(big.NewInt(0)) > 0)
}

// TestE2E_Consensus_EIP1559Check sends a legacy and a dynamic tx to the cluster
// and check if balance of sender, receiver, burn contract and miner is updates correctly
// in accordance with EIP-1559 specifications
func TestE2E_Consensus_EIP1559Check(t *testing.T) {
	sender1, err := wallet.GenerateKey()
	require.NoError(t, err)

	sender2, err := wallet.GenerateKey()
	require.NoError(t, err)

	recipientKey, err := wallet.GenerateKey()
	require.NoError(t, err)

	recipient := recipientKey.Address()

	// first account should have some matics premined
	cluster := framework.NewTestCluster(t, 5,
		framework.WithNativeTokenConfig(fmt.Sprintf(framework.NativeTokenMintableTestCfg, sender1.Address())),
		framework.WithPremine(types.Address(sender1.Address()), types.Address(sender2.Address())),
		framework.WithBurnContract(&polybft.BurnContractInfo{BlockNumber: 0, Address: types.ZeroAddress}),
	)
	defer cluster.Stop()

	cluster.WaitForReady(t)

	client := cluster.Servers[0].JSONRPC().Eth()

	waitUntilBalancesChanged := func(acct ethgo.Address, bal *big.Int) error {
		err := cluster.WaitUntil(30*time.Second, 2*time.Second, func() bool {
			balance, err := client.GetBalance(recipient, ethgo.Latest)
			if err != nil {
				return true
			}

			return balance.Cmp(bal) > 0
		})

		return err
	}

	relayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(cluster.Servers[0].JSONRPCAddr()))
	require.NoError(t, err)

	// create and send tx
	sendAmount := ethgo.Gwei(1)

	txn := []*ethgo.Transaction{
		{
			Value:    sendAmount,
			To:       &recipient,
			Gas:      21000,
			Nonce:    uint64(0),
			GasPrice: ethgo.Gwei(1).Uint64(),
		},
		{
			Value:                sendAmount,
			To:                   &recipient,
			Gas:                  21000,
			Nonce:                uint64(0),
			Type:                 ethgo.TransactionDynamicFee,
			MaxFeePerGas:         ethgo.Gwei(1),
			MaxPriorityFeePerGas: ethgo.Gwei(1),
		},
	}

	initialMinerBalance := big.NewInt(0)

	var prevMiner ethgo.Address

	for i := 0; i < 2; i++ {
		curTxn := txn[i]

		senderInitialBalance, _ := client.GetBalance(sender1.Address(), ethgo.Latest)
		receiverInitialBalance, _ := client.GetBalance(recipient, ethgo.Latest)
		burnContractInitialBalance, _ := client.GetBalance(ethgo.Address(types.ZeroAddress), ethgo.Latest)

		receipt, err := relayer.SendTransaction(curTxn, sender1)
		require.NoError(t, err)

		// wait for balance to get changed
		err = waitUntilBalancesChanged(recipient, receiverInitialBalance)
		require.NoError(t, err)

		// Retrieve the transaction receipt
		txReceipt, err := client.GetTransactionByHash(receipt.TransactionHash)
		require.NoError(t, err)

		block, _ := client.GetBlockByHash(txReceipt.BlockHash, true)
		finalMinerFinalBalance, _ := client.GetBalance(block.Miner, ethgo.Latest)

		if i == 0 {
			prevMiner = block.Miner
		}

		senderFinalBalance, _ := client.GetBalance(sender1.Address(), ethgo.Latest)
		receiverFinalBalance, _ := client.GetBalance(recipient, ethgo.Latest)
		burnContractFinalBalance, _ := client.GetBalance(ethgo.Address(types.ZeroAddress), ethgo.Latest)

		diffReciverBalance := new(big.Int).Sub(receiverFinalBalance, receiverInitialBalance)
		assert.Equal(t, sendAmount, diffReciverBalance, "Receiver balance should be increased by send amount")

		if i == 1 && prevMiner != block.Miner {
			initialMinerBalance = big.NewInt(0)
		}

		diffBurnContractBalance := new(big.Int).Sub(burnContractFinalBalance, burnContractInitialBalance)
		diffSenderBalance := new(big.Int).Sub(senderInitialBalance, senderFinalBalance)
		diffMinerBalance := new(big.Int).Sub(finalMinerFinalBalance, initialMinerBalance)

		diffSenderBalance.Sub(diffSenderBalance, diffReciverBalance)
		diffSenderBalance.Sub(diffSenderBalance, diffBurnContractBalance)
		diffSenderBalance.Sub(diffSenderBalance, diffMinerBalance)

		assert.Zero(t, diffSenderBalance.Int64(), "Sender balance should be decreased by send amount + gas")

		initialMinerBalance = finalMinerFinalBalance
	}
}
