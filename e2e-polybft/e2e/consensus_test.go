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
		validatorSetSize = 5
		epochSize        = 5
	)

	var (
		firstValidatorDataDir  = fmt.Sprintf("test-chain-%d", validatorSetSize+1) // directory where the first validator secrets will be stored
		secondValidatorDataDir = fmt.Sprintf("test-chain-%d", validatorSetSize+2) // directory where the second validator secrets will be stored

		premineBalance = ethgo.Ether(2e6) // 2M native tokens (so that we have enough balance to fund new validator)
	)

	// start cluster with 'validatorSize' validators
	cluster := framework.NewTestCluster(t, validatorSetSize,
		framework.WithEpochSize(epochSize),
		framework.WithEpochReward(int(ethgo.Ether(1).Uint64())),
		framework.WithSecretsCallback(func(addresses []types.Address, config *framework.TestClusterConfig) {
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

	polybftConfig, chainID, err := polybft.LoadPolyBFTConfig(path.Join(cluster.Config.TmpDir, chainConfigFileName))
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
	err = cluster.Bridge.FundSingleValidator(polybftConfig.Bridge.RootNativeERC20Addr, path.Join(cluster.Config.TmpDir, firstValidatorSecrets), initialBalance)
	require.NoError(t, err)

	// fund second new validator
	err = cluster.Bridge.FundSingleValidator(polybftConfig.Bridge.RootNativeERC20Addr, path.Join(cluster.Config.TmpDir, secondValidatorSecrets), initialBalance)
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
		cluster.Bridge.JSONRPCAddr(), true, false)

	cluster.InitTestServer(t, cluster.Config.ValidatorPrefix+strconv.Itoa(validatorSetSize+2),
		cluster.Bridge.JSONRPCAddr(), true, false)

	// collect the first and the second validator from the cluster
	firstValidator := cluster.Servers[validatorSetSize]
	secondValidator := cluster.Servers[validatorSetSize+1]

	initialStake := ethgo.Ether(499)

	// register the first validator with stake
	require.NoError(t, firstValidator.RegisterValidator(polybftConfig.Bridge.CustomSupernetManagerAddr))

	// register the second validator without stake
	require.NoError(t, secondValidator.RegisterValidator(polybftConfig.Bridge.CustomSupernetManagerAddr))

	// stake manually for the first validator
	require.NoError(t, firstValidator.Stake(polybftConfig, chainID, initialStake))

	// stake manually for the second validator
	require.NoError(t, secondValidator.Stake(polybftConfig, chainID, initialStake))

	firstValidatorInfo, err := sidechain.GetValidatorInfo(firstValidatorAddr,
		polybftConfig.Bridge.CustomSupernetManagerAddr, polybftConfig.Bridge.StakeManagerAddr,
		chainID, rootChainRelayer, childChainRelayer)
	require.NoError(t, err)
	require.True(t, firstValidatorInfo.IsActive)
	require.True(t, firstValidatorInfo.Stake.Cmp(initialStake) == 0)

	secondValidatorInfo, err := sidechain.GetValidatorInfo(secondValidatorAddr,
		polybftConfig.Bridge.CustomSupernetManagerAddr, polybftConfig.Bridge.StakeManagerAddr,
		chainID, rootChainRelayer, childChainRelayer)
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

	// wait for couple of epochs to have some rewards accumulated
	require.NoError(t, cluster.WaitForBlock(polybftConfig.EpochSize*7, time.Minute))

	bigZero := big.NewInt(0)

	firstValidatorInfo, err = sidechain.GetValidatorInfo(firstValidatorAddr,
		polybftConfig.Bridge.CustomSupernetManagerAddr, polybftConfig.Bridge.StakeManagerAddr,
		chainID, rootChainRelayer, childChainRelayer)
	require.NoError(t, err)
	require.True(t, firstValidatorInfo.IsActive)
	require.True(t, firstValidatorInfo.WithdrawableRewards.Cmp(bigZero) > 0)

	secondValidatorInfo, err = sidechain.GetValidatorInfo(secondValidatorAddr,
		polybftConfig.Bridge.CustomSupernetManagerAddr, polybftConfig.Bridge.StakeManagerAddr,
		chainID, rootChainRelayer, childChainRelayer)
	require.NoError(t, err)
	require.True(t, secondValidatorInfo.IsActive)
	require.True(t, secondValidatorInfo.WithdrawableRewards.Cmp(bigZero) > 0)
}

func TestE2E_Consensus_Validator_Unstake(t *testing.T) {
	premineAmount := ethgo.Ether(10)

	cluster := framework.NewTestCluster(t, 5,
		framework.WithEpochReward(int(ethgo.Ether(1).Uint64())),
		framework.WithEpochSize(5),
		framework.WithSecretsCallback(func(addresses []types.Address, config *framework.TestClusterConfig) {
			for _, a := range addresses {
				config.Premine = append(config.Premine, fmt.Sprintf("%s:%d", a, premineAmount))
				config.StakeAmounts = append(config.StakeAmounts, fmt.Sprintf("%s:%d", a, premineAmount))
			}
		}),
	)

	polybftCfg, chainID, err := polybft.LoadPolyBFTConfig(path.Join(cluster.Config.TmpDir, chainConfigFileName))
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
		chainID, rootChainRelayer, childChainRelayer)
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

	// send exit transaction to exit helper
	err = cluster.Bridge.SendExitTransaction(polybftCfg.Bridge.ExitHelperAddr, 1, srv.BridgeJSONRPCAddr(), srv.JSONRPCAddr())
	require.NoError(t, err)

	// make sure exit event is processed successfully
	isProcessed, err := isExitEventProcessed(1, ethgo.Address(polybftCfg.Bridge.ExitHelperAddr), rootChainRelayer)
	require.NoError(t, err)
	require.True(t, isProcessed, "exit event with was not processed")

	// check that validator is no longer active (out of validator set)
	validatorInfo, err = sidechain.GetValidatorInfo(validatorAddr,
		polybftCfg.Bridge.CustomSupernetManagerAddr, polybftCfg.Bridge.StakeManagerAddr,
		chainID, rootChainRelayer, childChainRelayer)
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
	initialBalance := ethgo.Ether(100000)

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
		require.Equal(t, new(big.Int).Add(mintAmount, initialBalance), balance)
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

func TestE2E_Consensus_CustomRewardToken(t *testing.T) {
	const epochSize = 5

	cluster := framework.NewTestCluster(t, 5,
		framework.WithEpochSize(epochSize),
		framework.WithEpochReward(1000000),
		framework.WithTestRewardToken(),
	)
	defer cluster.Stop()

	// wait for couple of epochs to accumulate some rewards
	require.NoError(t, cluster.WaitForBlock(epochSize*3, 3*time.Minute))

	// first validator is the owner of ChildValidator set smart contract
	owner := cluster.Servers[0]
	childChainRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(owner.JSONRPCAddr()))
	require.NoError(t, err)

	rootChainRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(cluster.Bridge.JSONRPCAddr()))
	require.NoError(t, err)

	polybftConfig, chainID, err := polybft.LoadPolyBFTConfig(path.Join(cluster.Config.TmpDir, chainConfigFileName))
	require.NoError(t, err)

	validatorAcc, err := sidechain.GetAccountFromDir(owner.DataDir())
	require.NoError(t, err)

	validatorInfo, err := sidechain.GetValidatorInfo(validatorAcc.Ecdsa.Address(),
		polybftConfig.Bridge.CustomSupernetManagerAddr, polybftConfig.Bridge.StakeManagerAddr,
		chainID, rootChainRelayer, childChainRelayer)
	t.Logf("[Validator#%v] Witdhrawable rewards=%d\n", validatorInfo.Address, validatorInfo.WithdrawableRewards)

	require.NoError(t, err)
	require.True(t, validatorInfo.WithdrawableRewards.Cmp(big.NewInt(0)) > 0)
}
