package e2e

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
	"github.com/0xPolygon/polygon-edge/helper/tests"
	"github.com/0xPolygon/polygon-edge/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
	"github.com/umbracle/ethgo/jsonrpc"
	ethgow "github.com/umbracle/ethgo/wallet"
)

var stateSyncResultEvent = abi.MustNewEvent(`event StateSyncResult(
		uint256 indexed counter,
		uint8 indexed status,
		bytes32 message)`)

type ResultEventStatus uint8

const (
	ResultEventSuccess ResultEventStatus = iota
	ResultEventFailure
)

// checkLogs is helper function which parses given ResultEvent event's logs,
// extracts status topic value and makes assertions against it.
func checkLogs(
	t *testing.T,
	logs []*ethgo.Log,
	expectedCount int,
	assertFn func(i int, status ResultEventStatus) bool,
) {
	t.Helper()
	require.Len(t, logs, expectedCount)

	for i, log := range logs {
		res, err := stateSyncResultEvent.ParseLog(log)
		assert.NoError(t, err)

		t.Logf("Block Number=%d, Decoded Log=%v", log.BlockNumber, res)

		status, ok := res["status"].(uint8)
		require.True(t, ok)

		assert.True(t, assertFn(i, ResultEventStatus(status)))
	}
}

func stateSyncEventToAbiSlice(stateSyncEvent types.StateSyncEvent) []map[string]interface{} {
	result := make([]map[string]interface{}, 1)
	result[0] = map[string]interface{}{
		"id":       stateSyncEvent.ID,
		"sender":   stateSyncEvent.Sender,
		"receiver": stateSyncEvent.Receiver,
		"data":     stateSyncEvent.Data,
		"skip":     stateSyncEvent.Skip,
	}

	return result
}

func executeStateSync(t *testing.T, client *jsonrpc.Client, account ethgo.Key, stateSyncID string) {
	t.Helper()

	// retrieve state sync proof
	var stateSyncProof types.StateSyncProof
	err := client.Call("bridge_getStateSyncProof", &stateSyncProof, stateSyncID)
	require.NoError(t, err)

	t.Log("State sync proofs:", stateSyncProof)

	input, err := types.ExecuteBundleABIMethod.Encode([2]interface{}{stateSyncProof.Proof, stateSyncEventToAbiSlice(stateSyncProof.StateSync)})
	require.NoError(t, err)

	t.Log(stateSyncEventToAbiSlice(stateSyncProof.StateSync))

	nonce, err := client.Eth().GetNonce(account.Address(), ethgo.Latest)
	require.NoError(t, err)

	// execute the state sync
	rawTxn := &ethgo.Transaction{
		From:     account.Address(),
		To:       (*ethgo.Address)(&contracts.StateReceiverContract),
		GasPrice: 0,
		Gas:      types.StateTransactionGasLimit,
		Input:    input,
		Nonce:    nonce,
	}

	chID, err := client.Eth().ChainID()
	require.NoError(t, err)

	signer := ethgow.NewEIP155Signer(chID.Uint64())
	signedTxn, err := signer.SignTx(rawTxn, account)
	require.NoError(t, err)

	txnRaw, err := signedTxn.MarshalRLPTo(nil)
	require.NoError(t, err)

	hash, err := client.Eth().SendRawTransaction(txnRaw)
	require.NoError(t, err)

	t.Log("Waiting for receipt", hash)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	receipt, err := tests.WaitForReceipt(ctx, client.Eth(), hash)
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
	require.NoError(t, cluster.WaitForBlock(25, 1*time.Minute))

	// commitments should've been stored
	// execute the state sysncs
	for i := 0; i < num; i++ {
		executeStateSync(t, cluster.Servers[0].JSONRPC(), accounts[i], fmt.Sprintf("%x", i+1))
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
	checkLogs(t, logs, num,
		func(_ int, status ResultEventStatus) bool {
			return status == ResultEventSuccess
		})
}
