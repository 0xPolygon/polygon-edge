package e2e

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"path"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/command/genesis"
	rootchainHelper "github.com/0xPolygon/polygon-edge/command/rootchain/helper"
	"github.com/0xPolygon/polygon-edge/command/sidechain"
	"github.com/0xPolygon/polygon-edge/consensus/polybft"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi/artifact"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
	ethgow "github.com/umbracle/ethgo/wallet"
)

const (
	manifestFileName = "manifest.json"
)

// checkLogs is helper function which parses given ResultEvent event's logs,
// extracts status topic value and makes assertions against it.
func checkLogs(
	t *testing.T,
	logs []*ethgo.Log,
	expectedCount int,
) {
	t.Helper()
	require.Len(t, logs, expectedCount)

	for _, log := range logs {
		stateSyncResultEvent := &contractsapi.StateSyncResultEvent{}
		assert.NoError(t, stateSyncResultEvent.ParseLog(log))

		t.Logf("Block Number=%d, Decoded Log=%+v", log.BlockNumber, stateSyncResultEvent)

		assert.True(t, stateSyncResultEvent.Status)
	}
}

func TestE2E_Bridge_MainWorkflow(t *testing.T) {
	const num = 10

	var (
		accounts         = make([]ethgo.Key, num)
		wallets, amounts [num]string
		premine          [num]types.Address
	)

	for i := 0; i < num; i++ {
		accounts[i], _ = ethgow.GenerateKey()
		premine[i] = types.Address(accounts[i].Address())
		wallets[i] = premine[i].String()
		amounts[i] = fmt.Sprintf("%d", 100)
	}

	cluster := framework.NewTestCluster(t, 5, framework.WithBridge(), framework.WithPremine(premine[:]...))
	defer cluster.Stop()

	// wait for a couple of blocks
	require.NoError(t, cluster.WaitForBlock(2, 1*time.Minute))

	// send a few transactions to the bridge
	require.NoError(
		t,
		cluster.EmitTransfer(
			contracts.NativeTokenContract.String(),
			strings.Join(wallets[:], ","),
			strings.Join(amounts[:], ","),
		),
	)

	// wait for a few more sprints
	require.NoError(t, cluster.WaitForBlock(35, 2*time.Minute))

	// the transactions are mined and there should be a success events
	id := contractsapi.StateReceiver.Abi.Events["StateSyncResult"].ID()
	filter := &ethgo.LogFilter{
		Topics: [][]*ethgo.Hash{
			{&id},
		},
	}

	filter.SetFromUint64(0)
	filter.SetToUint64(100)

	logs, err := cluster.Servers[0].JSONRPC().Eth().GetLogs(filter)
	require.NoError(t, err)

	// Assert that all state syncs are executed successfully
	checkLogs(t, logs, num)
}

func TestE2E_Bridge_MultipleCommitmentsPerEpoch(t *testing.T) {
	const num = 10

	var (
		accounts         = make([]ethgo.Key, num)
		wallets, amounts [num]string
		premine          [num]types.Address
	)

	for i := 0; i < num; i++ {
		accounts[i], _ = ethgow.GenerateKey()
		premine[i] = types.Address(accounts[i].Address())
		wallets[i] = premine[i].String()
		amounts[i] = fmt.Sprintf("%d", 100)
	}

	cluster := framework.NewTestCluster(t, 5, framework.WithBridge(), framework.WithPremine(premine[:]...), framework.WithEpochSize(30))
	defer cluster.Stop()

	// wait for a couple of blocks
	require.NoError(t, cluster.WaitForBlock(2, 1*time.Minute))

	// send two transactions to the bridge so that we have a minimal commitment
	require.NoError(
		t,
		cluster.EmitTransfer(
			contracts.NativeTokenContract.String(),
			strings.Join(wallets[:2], ","),
			strings.Join(amounts[:2], ","),
		),
	)

	// wait for a few more sprints
	require.NoError(t, cluster.WaitForBlock(10, 2*time.Minute))

	client := cluster.Servers[0].JSONRPC()
	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithClient(client))
	require.NoError(t, err)

	lastCommittedIDMethod := contractsapi.StateReceiver.Abi.GetMethod("lastCommittedId")
	encode, err := lastCommittedIDMethod.Encode([]interface{}{})
	require.NoError(t, err)

	// check that we submitted the minimal commitment to smart contract
	result, err := txRelayer.Call(accounts[0].Address(), ethgo.Address(contracts.StateReceiverContract), encode)
	require.NoError(t, err)

	lastCommittedID, err := strconv.ParseUint(result, 0, 64)
	require.NoError(t, err)
	require.Equal(t, uint64(2), lastCommittedID)

	// send some more transactions to the bridge to build another commitment in epoch
	require.NoError(
		t,
		cluster.EmitTransfer(
			contracts.NativeTokenContract.String(),
			strings.Join(wallets[2:], ","),
			strings.Join(amounts[2:], ","),
		),
	)

	// wait for a few more sprints
	require.NoError(t, cluster.WaitForBlock(40, 3*time.Minute))

	// check that we submitted the minimal commitment to smart contract
	result, err = txRelayer.Call(accounts[0].Address(), ethgo.Address(contracts.StateReceiverContract), encode)
	require.NoError(t, err)

	// check that the second (larger commitment) was also submitted in epoch
	lastCommittedID, err = strconv.ParseUint(result, 0, 64)
	require.NoError(t, err)
	require.Equal(t, uint64(10), lastCommittedID)

	// the transactions are mined and state syncs should be executed by the relayer
	// and there should be a success events
	id := contractsapi.StateReceiver.Abi.Events["StateSyncResult"].ID()
	filter := &ethgo.LogFilter{
		Topics: [][]*ethgo.Hash{
			{&id},
		},
	}

	filter.SetFromUint64(0)
	filter.SetToUint64(100)

	logs, err := cluster.Servers[0].JSONRPC().Eth().GetLogs(filter)
	require.NoError(t, err)

	// Assert that all state syncs are executed successfully
	checkLogs(t, logs, num)
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
	rootchainSender := ethgo.Address(manifest.RootchainConfig.AdminAddress)

	testCheckpointBlockNumber := func(expectedCheckpointBlock uint64) (bool, error) {
		actualCheckpointBlock, err := getCheckpointBlockNumber(l1Relayer, checkpointManagerAddr, rootchainSender)
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
	cluster.Bridge.Start()

	// check if pending checkpoint blocks were submitted (namely the last checkpointed block must be block 20)
	err = cluster.Bridge.WaitUntil(2*time.Second, 50*time.Second, func() (bool, error) {
		return testCheckpointBlockNumber(20)
	})
	require.NoError(t, err)
}

// getCheckpointBlockNumber gets current checkpoint block number from checkpoint manager smart contract
func getCheckpointBlockNumber(l1Relayer txrelayer.TxRelayer, checkpointManagerAddr, sender ethgo.Address) (uint64, error) {
	checkpointBlockNumRaw, err := ABICall(l1Relayer, contractsapi.CheckpointManager,
		checkpointManagerAddr, sender, "currentCheckpointBlockNumber")
	if err != nil {
		return 0, err
	}

	actualCheckpointBlock, err := types.ParseUint64orHex(&checkpointBlockNumRaw)
	if err != nil {
		return 0, err
	}

	return actualCheckpointBlock, nil
}

func TestE2E_Bridge_L2toL1Exit(t *testing.T) {
	const (
		userNumber      = 10
		epochSize       = 30
		checkpointBlock = uint64(epochSize)
		checkpointEpoch = uint64(1)
	)

	sidechainKeys := make([]*ethgow.Key, userNumber)
	accountAddress := make([]types.Address, userNumber)

	for i := 0; i < userNumber; i++ {
		key, err := ethgow.GenerateKey()
		require.NoError(t, err)

		sidechainKeys[i] = key
		accountAddress[i] = types.Address(key.Address())
	}

	// initialize rootchain admin key to default one
	require.NoError(t, rootchainHelper.InitRootchainAdminKey(""))

	cluster := framework.NewTestCluster(t, 5,
		framework.WithBridge(),
		framework.WithPremine(accountAddress...),
		framework.WithEpochSize(epochSize),
	)

	defer cluster.Stop()

	manifest, err := polybft.LoadManifest(path.Join(cluster.Config.TmpDir, manifestFileName))
	require.NoError(t, err)

	checkpointManagerAddr := ethgo.Address(manifest.RootchainConfig.CheckpointManagerAddress)
	exitHelperAddr := ethgo.Address(manifest.RootchainConfig.ExitHelperAddress)
	adminAddr := ethgo.Address(manifest.RootchainConfig.AdminAddress)

	// wait for a couple of blocks
	require.NoError(t, cluster.WaitForBlock(2, 2*time.Minute))

	// init rpc clients
	l1TxRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(txrelayer.DefaultRPCAddress))
	require.NoError(t, err)
	l2TxRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(cluster.Servers[0].JSONRPCAddr()))
	require.NoError(t, err)

	// deploy L1ExitTest contract
	receipt, err := l1TxRelayer.SendTransaction(&ethgo.Transaction{Input: contractsapi.TestL1StateReceiver.Bytecode},
		rootchainHelper.GetRootchainAdminKey())
	require.NoError(t, err)
	require.Equal(t, receipt.Status, uint64(types.ReceiptSuccess))

	l1ExitTestAddr := receipt.ContractAddress
	l2StateSenderAddress := ethgo.Address(contracts.L2StateSenderContract)

	// Start test
	// send crosschain transaction on l2 and get exit id
	stateSenderData := []byte{123}
	for i := 0; i < userNumber; i++ {
		receipt, err := ABITransaction(l2TxRelayer, sidechainKeys[i], contractsapi.L2StateSender, l2StateSenderAddress, "syncState", l1ExitTestAddr, stateSenderData)
		require.NoError(t, err)
		require.Equal(t, receipt.Status, uint64(types.ReceiptSuccess))
	}

	require.NoError(t, cluster.WaitForBlock(epochSize, 2*time.Minute))

	fail := 0

	for range time.Tick(time.Second) {
		currentEpochString, err := ABICall(l1TxRelayer, contractsapi.CheckpointManager, checkpointManagerAddr, adminAddr, "currentEpoch")
		require.NoError(t, err)

		currentEpoch, err := types.ParseUint64orHex(&currentEpochString)
		require.NoError(t, err)

		if currentEpoch >= checkpointEpoch {
			break
		}

		if fail > 300 {
			t.Fatal("epoch havent achieved")
		}
		fail++
	}

	var proof types.Proof

	for i := 0; i < userNumber; i++ {
		exitID := uint64(i + 1) // because exit events start from ID = 1
		proof, err = getExitProof(cluster.Servers[0].JSONRPCAddr(), exitID, checkpointEpoch, checkpointBlock)
		require.NoError(t, err)

		isProcessed, err := isExitEventProcessed(sidechainKeys[i], proof, checkpointBlock, stateSenderData, l1ExitTestAddr, exitHelperAddr, adminAddr, l1TxRelayer, exitID)
		require.NoError(t, err)
		require.True(t, isProcessed)
	}
}

func TestE2E_Bridge_L2toL1ExitMultiple(t *testing.T) {
	const (
		userNumber      = 6
		epochSize       = 10
		roundNumber     = 3
		checkpointBlock = uint64(epochSize)
		checkpointEpoch = uint64(1)
	)

	exitEventIds := make([]uint64, userNumber*roundNumber)

	sidechainKeys := make([]*ethgow.Key, userNumber)
	accountAddress := make([]types.Address, userNumber)

	for i := 0; i < userNumber; i++ {
		key, err := ethgow.GenerateKey()
		require.NoError(t, err)

		sidechainKeys[i] = key
		accountAddress[i] = types.Address(key.Address())
	}

	// initialize rootchain admin key to default one
	require.NoError(t, rootchainHelper.InitRootchainAdminKey(""))

	cluster := framework.NewTestCluster(t, 5,
		framework.WithBridge(),
		framework.WithPremine(accountAddress...),
		framework.WithEpochSize(epochSize),
	)

	defer cluster.Stop()

	manifest, err := polybft.LoadManifest(path.Join(cluster.Config.TmpDir, manifestFileName))
	require.NoError(t, err)

	checkpointManagerAddr := ethgo.Address(manifest.RootchainConfig.CheckpointManagerAddress)
	exitHelperAddr := ethgo.Address(manifest.RootchainConfig.ExitHelperAddress)
	adminAddr := ethgo.Address(manifest.RootchainConfig.AdminAddress)

	// wait for a couple of blocks
	require.NoError(t, cluster.WaitForBlock(2, 2*time.Minute))

	// init rpc clients
	l1TxRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(txrelayer.DefaultRPCAddress))
	require.NoError(t, err)
	l2TxRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(cluster.Servers[0].JSONRPCAddr()))
	require.NoError(t, err)

	// deploy L1ExitTest contract
	receipt, err := l1TxRelayer.SendTransaction(&ethgo.Transaction{Input: contractsapi.TestL1StateReceiver.Bytecode},
		rootchainHelper.GetRootchainAdminKey())
	require.NoError(t, err)
	require.Equal(t, receipt.Status, uint64(types.ReceiptSuccess))

	l1ExitTestAddr := receipt.ContractAddress
	l2StateSenderAddress := ethgo.Address(contracts.L2StateSenderContract)

	// Start test
	// send crosschain transaction on l2 and get exit id
	stateSenderData := []byte{123}
	//g, _ := errgroup.WithContext(context.Background())
	addTransaction := func(j, i uint64) {
		receipt, err := ABITransaction(l2TxRelayer, sidechainKeys[j], contractsapi.L2StateSender, l2StateSenderAddress, "syncState", l1ExitTestAddr, stateSenderData)
		require.NoError(t, err)
		require.Equal(t, receipt.Status, uint64(types.ReceiptSuccess))

		eventData, err := contractsapi.L2StateSender.Abi.Events["L2StateSynced"].ParseLog(receipt.Logs[0])
		require.NoError(t, err)

		exitEventIds[j+(i-1)*userNumber] = eventData["id"].(*big.Int).Uint64() //nolint:forcetypeassert
	}

	for i := 1; i <= roundNumber; i++ {
		for j := 0; j < userNumber; j++ {
			go addTransaction(uint64(j), uint64(i))
		}
		require.NoError(t, cluster.WaitForBlock(uint64((i)*epochSize), 2*time.Minute))

		fail := 0

		for range time.Tick(time.Second) {
			currentEpochString, err := ABICall(l1TxRelayer, contractsapi.CheckpointManager, checkpointManagerAddr, adminAddr, "currentEpoch")
			require.NoError(t, err)

			currentEpoch, err := types.ParseUint64orHex(&currentEpochString)
			require.NoError(t, err)

			if currentEpoch >= uint64(i)*checkpointEpoch {
				break
			}

			if fail > 300 {
				t.Fatal("epoch havent achieved")
			}
			fail++
		}
	}

	var proof types.Proof

	for i := 0; i < roundNumber; i++ {
		for j := 0; j < userNumber; j++ {
			proof, err = getExitProof(cluster.Servers[0].JSONRPCAddr(), exitEventIds[j+i*userNumber], uint64(i+1)*checkpointEpoch, uint64(i+1)*checkpointBlock)
			require.NoError(t, err)
			isProcessed, err := isExitEventProcessed(sidechainKeys[j], proof, uint64(i+1)*checkpointBlock, stateSenderData, l1ExitTestAddr, exitHelperAddr, adminAddr, l1TxRelayer, exitEventIds[j+i*userNumber])
			require.NoError(t, err)
			require.True(t, isProcessed)
		}
	}
}

func isExitEventProcessed(sidechainKey *ethgow.Key, proof types.Proof, checkpointBlock uint64, stateSenderData []byte, l1ExitTestAddr, exitHelperAddr, adminAddr ethgo.Address, l1TxRelayer txrelayer.TxRelayer, exitEventID uint64) (bool, error) {
	proofExitEventEncoded, err := polybft.ExitEventABIType.Encode(&polybft.ExitEvent{
		ID:       exitEventID,
		Sender:   sidechainKey.Address(),
		Receiver: l1ExitTestAddr,
		Data:     stateSenderData,
	})
	if err != nil {
		return false, err
	}

	leafIndex, ok := proof.Metadata["LeafIndex"].(float64)
	if !ok {
		return false, fmt.Errorf("could not get leaf index from exit event proof. Leaf from proof: %v", proof.Metadata["LeafIndex"])
	}

	receipt, err := ABITransaction(l1TxRelayer, rootchainHelper.GetRootchainAdminKey(), contractsapi.ExitHelper, exitHelperAddr,
		"exit",
		big.NewInt(int64(checkpointBlock)),
		uint64(leafIndex),
		proofExitEventEncoded,
		proof.Data,
	)

	if err != nil {
		return false, err
	}

	if receipt.Status != uint64(types.ReceiptSuccess) {
		return false, errors.New("transaction execution failed")
	}

	result, err := ABICall(l1TxRelayer, contractsapi.ExitHelper, exitHelperAddr, adminAddr, "processedExits", big.NewInt(int64(exitEventID)))
	if err != nil {
		return false, err
	}

	parserRes, err := types.ParseUint64orHex(&result)
	if err != nil {
		return false, err
	}

	return parserRes == uint64(1), nil
}

func getExitProof(rpcAddress string, exitID, epoch, checkpointBlock uint64) (types.Proof, error) {
	query := struct {
		Jsonrpc string   `json:"jsonrpc"`
		Method  string   `json:"method"`
		Params  []string `json:"params"`
		ID      int      `json:"id"`
	}{
		"2.0",
		"bridge_generateExitProof",
		[]string{fmt.Sprintf("0x%x", exitID), fmt.Sprintf("0x%x", epoch), fmt.Sprintf("0x%x", checkpointBlock)},
		1,
	}

	d, err := json.Marshal(query)
	if err != nil {
		return types.Proof{}, err
	}

	resp, err := http.Post(rpcAddress, "application/json", bytes.NewReader(d))
	if err != nil {
		return types.Proof{}, err
	}

	s, err := io.ReadAll(resp.Body)
	if err != nil {
		return types.Proof{}, err
	}

	rspProof := struct {
		Result types.Proof `json:"result"`
	}{}

	err = json.Unmarshal(s, &rspProof)
	if err != nil {
		return types.Proof{}, err
	}

	return rspProof.Result, nil
}

// TODO: Remove this to some separate file, containing helper functions?
func ABICall(relayer txrelayer.TxRelayer, artifact *artifact.Artifact, contractAddress ethgo.Address, senderAddr ethgo.Address, method string, params ...interface{}) (string, error) {
	input, err := artifact.Abi.GetMethod(method).Encode(params)
	if err != nil {
		return "", err
	}

	return relayer.Call(senderAddr, contractAddress, input)
}

func ABITransaction(relayer txrelayer.TxRelayer, key ethgo.Key, artifact *artifact.Artifact, contractAddress ethgo.Address, method string, params ...interface{}) (*ethgo.Receipt, error) {
	input, err := artifact.Abi.GetMethod(method).Encode(params)
	if err != nil {
		return nil, err
	}

	return relayer.SendTransaction(&ethgo.Transaction{
		To:    &contractAddress,
		Input: input,
	}, key)
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

	l1Sender := ethgo.Address(manifest.RootchainConfig.AdminAddress)
	// assert that block 20 gets checkpointed
	require.NoError(t, cluster.Bridge.WaitUntil(time.Second, time.Minute, func() (bool, error) {
		actualCheckpointBlock, err := getCheckpointBlockNumber(l1Relayer, checkpointManagerAddr, l1Sender)
		if err != nil {
			return false, err
		}

		t.Logf("Checkpoint block: %d\n", actualCheckpointBlock)

		return actualCheckpointBlock == finalBlockNumber, nil
	}))
}
