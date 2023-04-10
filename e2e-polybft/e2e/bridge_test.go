package e2e

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"path"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
	ethgow "github.com/umbracle/ethgo/wallet"

	"github.com/0xPolygon/polygon-edge/command/genesis"
	"github.com/0xPolygon/polygon-edge/command/sidechain"
	"github.com/0xPolygon/polygon-edge/consensus/polybft"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"

	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
)

const (
	manifestFileName = "manifest.json"
)

func TestE2E_Bridge_DepositAndWithdrawERC20(t *testing.T) {
	const (
		num                   = 10
		amount                = 100
		numBlockConfirmations = 2
		// make epoch size long enough, so that all exit events are processed within the same epoch
		epochSize = 30
	)

	receivers := make([]string, num)
	amounts := make([]string, num)

	for i := 0; i < num; i++ {
		key, err := ethgow.GenerateKey()
		require.NoError(t, err)

		receivers[i] = types.Address(key.Address()).String()
		amounts[i] = fmt.Sprintf("%d", amount)

		t.Logf("Receiver#%d=%s\n", i+1, receivers[i])
	}

	cluster := framework.NewTestCluster(t, 5,
		framework.WithBridge(),
		framework.WithNumBlockConfirmations(numBlockConfirmations),
		framework.WithEpochSize(epochSize))
	defer cluster.Stop()

	cluster.WaitForReady(t)

	manifest, err := polybft.LoadManifest(path.Join(cluster.Config.TmpDir, manifestFileName))
	require.NoError(t, err)

	// DEPOSIT ERC20 TOKENS
	// send a few transactions to the bridge
	require.NoError(
		t,
		cluster.Bridge.DepositERC20(
			manifest.RootchainConfig.RootNativeERC20Address,
			manifest.RootchainConfig.RootERC20PredicateAddress,
			strings.Join(receivers[:], ","),
			strings.Join(amounts[:], ","),
		),
	)

	// wait for a few more sprints
	require.NoError(t, cluster.WaitForBlock(35, 2*time.Minute))

	// the transactions are processed and there should be a success events
	var stateSyncedResult contractsapi.StateSyncResultEvent

	id := stateSyncedResult.Sig()
	filter := &ethgo.LogFilter{
		Topics: [][]*ethgo.Hash{
			{&id},
		},
	}

	filter.SetFromUint64(0)
	filter.SetToUint64(100)

	childEthEndpoint := cluster.Servers[0].JSONRPC().Eth()

	logs, err := childEthEndpoint.GetLogs(filter)
	require.NoError(t, err)

	// assert that all deposits are executed successfully
	checkStateSyncResultLogs(t, logs, num)

	// check receivers balances got increased by deposited amount
	for _, receiver := range receivers {
		balance, err := childEthEndpoint.GetBalance(ethgo.Address(types.StringToAddress(receiver)), ethgo.Latest)
		require.NoError(t, err)
		require.Equal(t, big.NewInt(amount), balance)
	}

	t.Log("Deposits were successfully processed")

	// WITHDRAW ERC20 TOKENS
	rootchainTxRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(cluster.Bridge.JSONRPCAddr()))
	require.NoError(t, err)

	senderAccount, err := sidechain.GetAccountFromDir(cluster.Servers[0].DataDir())
	require.NoError(t, err)

	t.Logf("Withdraw sender: %s\n", senderAccount.Ecdsa.Address())

	rawKey, err := senderAccount.Ecdsa.MarshallPrivateKey()
	require.NoError(t, err)

	// send withdraw transaction
	err = cluster.Bridge.WithdrawERC20(
		hex.EncodeToString(rawKey),
		strings.Join(receivers[:], ","),
		strings.Join(amounts[:], ","),
		cluster.Servers[0].JSONRPCAddr())
	require.NoError(t, err)

	currentBlock, err := childEthEndpoint.GetBlockByNumber(ethgo.Latest, false)
	require.NoError(t, err)

	currentExtra, err := polybft.GetIbftExtra(currentBlock.ExtraData)
	require.NoError(t, err)

	t.Logf("Latest block number: %d, epoch number: %d\n", currentBlock.Number, currentExtra.Checkpoint.EpochNumber)

	currentEpoch := currentExtra.Checkpoint.EpochNumber
	fail := 0

	// make sure we have progressed to the next epoch on the root chain
	// before sending exit transaction to the root chain
	// (it means that checkpoint was submitted)
	for range time.Tick(time.Second) {
		currentEpochString, err := ABICall(rootchainTxRelayer, contractsapi.CheckpointManager,
			ethgo.Address(manifest.RootchainConfig.CheckpointManagerAddress), ethgo.ZeroAddress, "currentEpoch")
		require.NoError(t, err)

		rootchainEpoch, err := types.ParseUint64orHex(&currentEpochString)
		require.NoError(t, err)

		if rootchainEpoch >= currentEpoch {
			break
		}

		if fail > 300 {
			t.Fatal("root chain hasn't progressed to the next epoch")
		}
		fail++
	}

	exitHelper := manifest.RootchainConfig.ExitHelperAddress
	rootJSONRPC := cluster.Bridge.JSONRPCAddr()
	childJSONRPC := cluster.Servers[0].JSONRPCAddr()

	for i := uint64(0); i < num; i++ {
		exitEventID := i + 1

		// send exit transaction to exit helper
		err = cluster.Bridge.SendExitTransaction(exitHelper, exitEventID, rootJSONRPC, childJSONRPC)
		require.NoError(t, err)

		// make sure exit event is processed successfully
		isProcessed, err := isExitEventProcessed(exitEventID, ethgo.Address(exitHelper), rootchainTxRelayer)
		require.NoError(t, err)
		require.True(t, isProcessed, fmt.Sprintf("exit event with ID %d was not processed", exitEventID))
	}

	// assert that receiver's balances on RootERC20 smart contract are expected
	for _, receiver := range receivers {
		balanceInput, err := contractsapi.RootERC20.Abi.Methods["balanceOf"].Encode([]interface{}{receiver})
		require.NoError(t, err)

		balanceRaw, err := rootchainTxRelayer.Call(ethgo.ZeroAddress,
			ethgo.Address(manifest.RootchainConfig.RootNativeERC20Address), balanceInput)
		require.NoError(t, err)

		balance, err := types.ParseUint256orHex(&balanceRaw)
		require.NoError(t, err)
		require.Equal(t, big.NewInt(amount), balance)
	}
}

func TestE2E_Bridge_MultipleCommitmentsPerEpoch(t *testing.T) {
	const depositsCount = 10

	receivers := make([]string, depositsCount)
	amounts := make([]string, depositsCount)

	for i := 0; i < depositsCount; i++ {
		key, err := ethgow.GenerateKey()
		require.NoError(t, err)

		receivers[i] = types.Address(key.Address()).String()
		amounts[i] = fmt.Sprintf("%d", 100)
	}

	cluster := framework.NewTestCluster(t, 5,
		framework.WithBridge(),
		framework.WithEpochSize(30))
	defer cluster.Stop()

	manifest, err := polybft.LoadManifest(path.Join(cluster.Config.TmpDir, manifestFileName))
	require.NoError(t, err)

	cluster.WaitForReady(t)

	// send two transactions to the bridge so that we have a minimal commitment
	require.NoError(
		t,
		cluster.Bridge.DepositERC20(
			manifest.RootchainConfig.RootNativeERC20Address,
			manifest.RootchainConfig.RootERC20PredicateAddress,
			strings.Join(receivers[:2], ","),
			strings.Join(amounts[:2], ","),
		),
	)

	// wait for a few more sprints
	require.NoError(t, cluster.WaitForBlock(10, 2*time.Minute))

	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithClient(cluster.Servers[0].JSONRPC()))
	require.NoError(t, err)

	lastCommittedIDMethod := contractsapi.StateReceiver.Abi.GetMethod("lastCommittedId")
	encode, err := lastCommittedIDMethod.Encode([]interface{}{})
	require.NoError(t, err)

	// check that we submitted the minimal commitment to smart contract
	result, err := txRelayer.Call(ethgo.ZeroAddress, ethgo.Address(contracts.StateReceiverContract), encode)
	require.NoError(t, err)

	lastCommittedID, err := strconv.ParseUint(result, 0, 64)
	require.NoError(t, err)
	require.Equal(t, uint64(2), lastCommittedID)

	// send some more transactions to the bridge to build another commitment in epoch
	require.NoError(
		t,
		cluster.Bridge.DepositERC20(
			manifest.RootchainConfig.RootNativeERC20Address,
			manifest.RootchainConfig.RootERC20PredicateAddress,
			strings.Join(receivers[2:], ","),
			strings.Join(amounts[2:], ","),
		),
	)

	// wait for a few more sprints
	require.NoError(t, cluster.WaitForBlock(40, 3*time.Minute))

	// check that we submitted the minimal commitment to smart contract
	result, err = txRelayer.Call(ethgo.ZeroAddress, ethgo.Address(contracts.StateReceiverContract), encode)
	require.NoError(t, err)

	// check that the second (larger commitment) was also submitted in epoch
	lastCommittedID, err = strconv.ParseUint(result, 0, 64)
	require.NoError(t, err)
	require.Equal(t, uint64(depositsCount), lastCommittedID)

	// the transactions are mined and state syncs should be executed by the relayer
	// and there should be a success events
	var stateSyncedResult contractsapi.StateSyncResultEvent

	id := stateSyncedResult.Sig()
	filter := &ethgo.LogFilter{
		Topics: [][]*ethgo.Hash{
			{&id},
		},
	}

	filter.SetFromUint64(0)
	filter.SetToUint64(100)

	logs, err := cluster.Servers[0].JSONRPC().Eth().GetLogs(filter)
	require.NoError(t, err)

	// assert that all state syncs are executed successfully
	checkStateSyncResultLogs(t, logs, depositsCount)
}

func TestE2E_CheckpointSubmission(t *testing.T) {
	// spin up a cluster with epoch size set to 5 blocks
	cluster := framework.NewTestCluster(t, 5, framework.WithBridge(), framework.WithEpochSize(5))
	defer cluster.Stop()

	// initialize tx relayer used to query CheckpointManager smart contract
	l1Relayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(cluster.Bridge.JSONRPCAddr()))
	require.NoError(t, err)

	manifest, err := polybft.LoadManifest(path.Join(cluster.Config.TmpDir, manifestFileName))
	require.NoError(t, err)

	checkpointManagerAddr := ethgo.Address(manifest.RootchainConfig.CheckpointManagerAddress)

	testCheckpointBlockNumber := func(expectedCheckpointBlock uint64) (bool, error) {
		actualCheckpointBlock, err := getCheckpointBlockNumber(l1Relayer, checkpointManagerAddr)
		if err != nil {
			return false, err
		}

		t.Logf("Checkpoint block: %d\n", actualCheckpointBlock)

		return actualCheckpointBlock == expectedCheckpointBlock, nil
	}

	// wait for a single epoch to be checkpointed
	require.NoError(t, cluster.WaitForBlock(7, 30*time.Second))

	// checking last checkpoint block before rootchain server stop
	err = cluster.Bridge.WaitUntil(2*time.Second, 30*time.Second, func() (bool, error) {
		return testCheckpointBlockNumber(5)
	})
	require.NoError(t, err)

	// stop rootchain server
	cluster.Bridge.Stop()

	// wait for a couple of epochs so that there are pending checkpoint (epoch-ending) blocks
	require.NoError(t, cluster.WaitForBlock(21, 2*time.Minute))

	// restart rootchain server
	require.NoError(t, cluster.Bridge.Start())

	// check if pending checkpoint blocks were submitted (namely the last checkpointed block must be block 20)
	err = cluster.Bridge.WaitUntil(2*time.Second, 50*time.Second, func() (bool, error) {
		return testCheckpointBlockNumber(20)
	})
	require.NoError(t, err)
}

// getCheckpointBlockNumber gets current checkpoint block number from checkpoint manager smart contract
func getCheckpointBlockNumber(l1Relayer txrelayer.TxRelayer, checkpointManagerAddr ethgo.Address) (uint64, error) {
	checkpointBlockNumRaw, err := ABICall(l1Relayer, contractsapi.CheckpointManager,
		checkpointManagerAddr, ethgo.ZeroAddress, "currentCheckpointBlockNumber")
	if err != nil {
		return 0, err
	}

	actualCheckpointBlock, err := types.ParseUint64orHex(&checkpointBlockNumRaw)
	if err != nil {
		return 0, err
	}

	return actualCheckpointBlock, nil
}

func TestE2E_Bridge_ChangeVotingPower(t *testing.T) {
	const (
		finalBlockNumber   = 20
		votingPowerChanges = 3
	)

	cluster := framework.NewTestCluster(t, 5,
		framework.WithBridge(),
		framework.WithEpochSize(5),
		framework.WithEpochReward(1000))
	defer cluster.Stop()

	// load manifest file
	manifest, err := polybft.LoadManifest(path.Join(cluster.Config.TmpDir, manifestFileName))
	require.NoError(t, err)

	checkpointManagerAddr := ethgo.Address(manifest.RootchainConfig.CheckpointManagerAddress)

	validatorSecretFiles, err := genesis.GetValidatorKeyFiles(cluster.Config.TmpDir, cluster.Config.ValidatorPrefix)
	require.NoError(t, err)

	votingPowerChangeValidators := make([]ethgo.Address, votingPowerChanges)

	for i := 0; i < votingPowerChanges; i++ {
		validator, err := sidechain.GetAccountFromDir(path.Join(cluster.Config.TmpDir, validatorSecretFiles[i]))
		require.NoError(t, err)

		votingPowerChangeValidators[i] = validator.Ecdsa.Address()
	}

	// L2 Tx relayer (for sending stake transaction and querying validator)
	l2Relayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(cluster.Servers[0].JSONRPCAddr()))
	require.NoError(t, err)

	// L1 Tx relayer (for querying checkpoints)
	l1Relayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(cluster.Bridge.JSONRPCAddr()))
	require.NoError(t, err)

	// waiting two epochs, so that some rewards get accumulated
	require.NoError(t, cluster.WaitForBlock(10, 1*time.Minute))

	queryValidators := func(handler func(idx int, validatorInfo *polybft.ValidatorInfo)) {
		for i, validatorAddr := range votingPowerChangeValidators {
			// query validator info
			validatorInfo, err := sidechain.GetValidatorInfo(validatorAddr, l2Relayer)
			require.NoError(t, err)

			handler(i, validatorInfo)
		}
	}

	originalValidatorStorage := make(map[ethgo.Address]*polybft.ValidatorInfo, votingPowerChanges)

	queryValidators(func(idx int, validator *polybft.ValidatorInfo) {
		t.Logf("[Validator#%d] Voting power (original)=%d, rewards=%d\n",
			idx+1, validator.TotalStake, validator.WithdrawableRewards)

		originalValidatorStorage[validator.Address] = validator

		// stake rewards
		require.NoError(t, cluster.Servers[idx].Stake(validator.WithdrawableRewards.Uint64()))
	})

	// wait a two more epochs, so that stake is registered and two more checkpoints are sent.
	// Blocks are still produced, although voting power is slightly changed.
	require.NoError(t, cluster.WaitForBlock(finalBlockNumber, 1*time.Minute))

	queryValidators(func(idx int, validator *polybft.ValidatorInfo) {
		t.Logf("[Validator#%d] Voting power (after stake)=%d\n", idx+1, validator.TotalStake)

		previousValidatorInfo := originalValidatorStorage[validator.Address]
		stakedAmount := new(big.Int).Add(previousValidatorInfo.WithdrawableRewards, previousValidatorInfo.TotalStake)

		// assert that total stake has increased by staked amount
		require.Equal(t, stakedAmount, validator.TotalStake)
	})

	require.NoError(t, cluster.Bridge.WaitUntil(time.Second, time.Minute, func() (bool, error) {
		actualCheckpointBlock, err := getCheckpointBlockNumber(l1Relayer, checkpointManagerAddr)
		if err != nil {
			return false, err
		}

		t.Logf("Checkpoint block: %d\n", actualCheckpointBlock)

		return actualCheckpointBlock == finalBlockNumber, nil
	}))
}
