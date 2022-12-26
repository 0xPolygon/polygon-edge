package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"path"
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
	"github.com/umbracle/ethgo/abi"
	"github.com/umbracle/ethgo/jsonrpc"
	ethgow "github.com/umbracle/ethgo/wallet"
)

var (
	stateSyncResultEvent = abi.MustNewEvent(`event StateSyncResult(
		uint256 indexed counter,
		bool indexed status,
		bytes message)`)
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
		res, err := stateSyncResultEvent.ParseLog(log)
		assert.NoError(t, err)

		t.Logf("Block Number=%d, Decoded Log=%v", log.BlockNumber, res)

		status, ok := res["status"].(bool)
		require.True(t, ok)

		assert.True(t, status)
	}
}

func executeStateSync(t *testing.T, client *jsonrpc.Client, txRelayer txrelayer.TxRelayer, account ethgo.Key, stateSyncID string) {
	t.Helper()

	// retrieve state sync proof
	var stateSyncProof types.StateSyncProof
	err := client.Call("bridge_getStateSyncProof", &stateSyncProof, stateSyncID)
	require.NoError(t, err)

	t.Log("State sync proofs:", stateSyncProof)

	input, err := stateSyncProof.EncodeAbi()
	require.NoError(t, err)

	t.Log(stateSyncProof.StateSync.ToMap())

	// execute the state sync
	txn := &ethgo.Transaction{
		From:     account.Address(),
		To:       (*ethgo.Address)(&contracts.StateReceiverContract),
		GasPrice: 0,
		Gas:      types.StateTransactionGasLimit,
		Input:    input,
	}

	receipt, err := txRelayer.SendTransaction(txn, account)
	require.NoError(t, err)
	require.NotNil(t, receipt)

	t.Log("Logs", len(receipt.Logs))
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
	require.NoError(t, cluster.WaitForBlock(30, 2*time.Minute))

	client := cluster.Servers[0].JSONRPC()
	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithClient(client))
	require.NoError(t, err)

	// commitments should've been stored
	// execute the state syncs
	for i := 0; i < num; i++ {
		executeStateSync(t, client, txRelayer, accounts[i], fmt.Sprintf("%x", i+1))
	}

	// the transactions are mined and there should be a success events
	id := stateSyncResultEvent.ID()
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

		return actualCheckpointBlock < expectedCheckpointBlock, nil
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
	sidechainKey, err := ethgow.GenerateKey()
	require.NoError(t, err)

	// initialize rootchain admin key to default one
	err = rootchainHelper.InitRootchainAdminKey("")
	require.NoError(t, err)

	cluster := framework.NewTestCluster(t, 5,
		framework.WithBridge(),
		framework.WithPremine(types.Address(sidechainKey.Address())),
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
	receipt, err := l1TxRelayer.SendTransaction(&ethgo.Transaction{Input: contractsapi.L1ExitTestBytecode},
		rootchainHelper.GetRootchainAdminKey())
	require.NoError(t, err)
	require.Equal(t, receipt.Status, uint64(types.ReceiptSuccess))

	l1ExitTestAddr := receipt.ContractAddress
	l2StateSenderAddress := ethgo.Address(contracts.L2StateSenderContract)

	// Start test
	// send crosschain transaction on l2 and get exit id
	stateSenderData := []byte{123}
	receipt, err = ABITransaction(l2TxRelayer, sidechainKey, contractsapi.L2StateSender, l2StateSenderAddress, "syncState", l1ExitTestAddr, stateSenderData)
	require.NoError(t, err)
	require.Equal(t, receipt.Status, uint64(types.ReceiptSuccess))
	l2SenderBlock := receipt.BlockNumber

	eventData, err := contractsapi.L2StateSender.Abi.Events["L2StateSynced"].ParseLog(receipt.Logs[0])
	require.NoError(t, err)

	l2syncID, ok := eventData["id"].(*big.Int)
	require.True(t, ok)

	l2SenderBlockData, err := cluster.Servers[0].JSONRPC().Eth().GetBlockByNumber(ethgo.BlockNumber(l2SenderBlock), true)
	require.NoError(t, err)
	extra, err := polybft.GetIbftExtra(l2SenderBlockData.ExtraData)
	require.NoError(t, err)

	receipt, err = ABITransaction(l2TxRelayer, sidechainKey, contractsapi.L2StateSender, l2StateSenderAddress, "syncState", l1ExitTestAddr, stateSenderData)
	require.Equal(t, receipt.Status, uint64(types.ReceiptSuccess))
	require.NoError(t, err)

	// wait when a new checkpoint will be accepted
	fail := 0

	for range time.Tick(time.Second) {
		currentEpochString, err := ABICall(l1TxRelayer, contractsapi.CheckpointManager, checkpointManagerAddr, adminAddr, "currentEpoch")
		require.NoError(t, err)

		currentEpoch, err := types.ParseUint64orHex(&currentEpochString)
		require.NoError(t, err)

		if currentEpoch >= extra.Checkpoint.EpochNumber {
			break
		}

		if fail > 300 {
			t.Fatal("epoch havent achieved")
		}
		fail++
	}

	proof, err := getExitProof(cluster.Servers[0].JSONRPCAddr(), l2syncID.Uint64(), extra.Checkpoint.EpochNumber, extra.Checkpoint.EpochNumber*10)
	require.NoError(t, err)

	proofExitEventEncoded, err := polybft.ExitEventABIType.Encode(&polybft.ExitEvent{
		ID:       1,
		Sender:   sidechainKey.Address(),
		Receiver: l1ExitTestAddr,
		Data:     stateSenderData,
	})
	require.NoError(t, err)

	receipt, err = ABITransaction(l1TxRelayer, rootchainHelper.GetRootchainAdminKey(), contractsapi.ExitHelper, exitHelperAddr,
		"exit",
		big.NewInt(int64(extra.Checkpoint.EpochNumber*10)),
		proof.LeafIndex,
		proofExitEventEncoded,
		proof.Proof,
	)
	require.NoError(t, err)
	require.Equal(t, receipt.Status, uint64(types.ReceiptSuccess))

	res, err := ABICall(l1TxRelayer, contractsapi.ExitHelper, exitHelperAddr, adminAddr, "processedExits", big.NewInt(1))
	require.NoError(t, err)
	parserRes, err := types.ParseUint64orHex(&res)
	require.NoError(t, err)
	require.Equal(t, parserRes, uint64(1))
}

func getExitProof(rpcAddress string, exitID, epoch, checkpointBlock uint64) (types.ExitProof, error) {
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
		return types.ExitProof{}, err
	}

	resp, err := http.Post(rpcAddress, "application/json", bytes.NewReader(d))
	if err != nil {
		return types.ExitProof{}, err
	}

	s, err := io.ReadAll(resp.Body)
	if err != nil {
		return types.ExitProof{}, err
	}

	rspProof := struct {
		Result types.ExitProof `json:"result"`
	}{}

	err = json.Unmarshal(s, &rspProof)
	if err != nil {
		return types.ExitProof{}, err
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

type validatorInfo struct {
	address    ethgo.Address
	rewards    *big.Int
	totalStake *big.Int
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

	queryValidators := func(handler func(idx int, validatorInfo *validatorInfo)) {
		for i, validatorAddr := range votingPowerChangeValidators {
			// query validator info
			validatorInfoRaw, err := sidechain.GetValidatorInfo(validatorAddr, l2Relayer)
			require.NoError(t, err)

			rewards := validatorInfoRaw["withdrawableRewards"].(*big.Int) //nolint:forcetypeassert
			totalStake := validatorInfoRaw["totalStake"].(*big.Int)       //nolint:forcetypeassert

			handler(i, &validatorInfo{address: validatorAddr, rewards: rewards, totalStake: totalStake})
		}
	}

	originalValidatorStorage := make(map[ethgo.Address]*validatorInfo, votingPowerChanges)

	queryValidators(func(idx int, validator *validatorInfo) {
		t.Logf("[Validator#%d] Voting power (original)=%d, rewards=%d\n",
			idx+1, validator.totalStake, validator.rewards)

		originalValidatorStorage[validator.address] = validator

		// stake rewards
		require.NoError(t, cluster.Servers[idx].Stake(validator.rewards.Uint64()))
	})

	// wait a two more epochs, so that stake is registered and two more checkpoints are sent.
	// Blocks are still produced, although voting power is slightly changed.
	require.NoError(t, cluster.WaitForBlock(finalBlockNumber, 1*time.Minute))

	queryValidators(func(idx int, validator *validatorInfo) {
		t.Logf("[Validator#%d] Voting power (after stake)=%d\n", idx+1, validator.totalStake)

		previousValidatorInfo := originalValidatorStorage[validator.address]
		stakedAmount := new(big.Int).Add(previousValidatorInfo.rewards, previousValidatorInfo.totalStake)

		// assert that total stake has increased by staked amount
		require.Equal(t, stakedAmount, validator.totalStake)
	})

	l1Sender := ethgo.Address(manifest.RootchainConfig.AdminAddress)
	// assert that block 20 gets checkpointed
	require.NoError(t, cluster.Bridge.WaitUntil(time.Second, time.Minute, func() (bool, error) {
		actualCheckpointBlock, err := getCheckpointBlockNumber(l1Relayer, checkpointManagerAddr, l1Sender)
		if err != nil {
			return false, err
		}

		t.Logf("Checkpoint block: %d\n", actualCheckpointBlock)

		// waiting until condition is true (namely when block 20 gets checkpointed)
		return actualCheckpointBlock < finalBlockNumber, nil
	}))
}
