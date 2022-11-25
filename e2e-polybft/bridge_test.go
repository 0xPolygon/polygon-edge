package e2e

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
	"github.com/0xPolygon/polygon-edge/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
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
	var stateSyncProof types.StateSyncProof

	// retrieve state sync proofs
	err := cluster.Servers[0].JSONRPC().Call("bridge_getStateSyncProof", &stateSyncProof, "1")
	require.NoError(t, err)

	t.Log(stateSyncProof)

	// execute the state sync
	rawTxn := &ethgo.Transaction{
		From:     ethgo.Address(premine[0]),
		To:       (*ethgo.Address)(&contracts.StateReceiverContract),
		GasPrice: 0,
		Gas:      types.StateTransactionGasLimit,
	}

	chID, err := cluster.Servers[0].JSONRPC().Eth().ChainID()
	require.NoError(t, err)

	signer := ethgow.NewEIP155Signer(chID.Uint64())
	signedTxn, err := signer.SignTx(rawTxn, accounts[0])
	assert.NoError(t, err)

	txnRaw, err := signedTxn.MarshalRLPTo(nil)
	assert.NoError(t, err)

	hash, err := cluster.Servers[0].JSONRPC().Eth().SendRawTransaction(txnRaw)
	assert.NoError(t, err)

	var receipt *ethgo.Receipt
	for count := 0; count < 100 || receipt == nil; count++ {
		receipt, err = cluster.Servers[0].JSONRPC().Eth().GetTransactionReceipt(hash)
		assert.NoError(t, err)

		time.Sleep(50 * time.Millisecond)
	}
	require.NotNil(t, receipt)
	t.Log(receipt.Logs)

	// the transaction is mined and there should be a success event
	id := stateSyncResultEvent.ID()
	filter := &ethgo.LogFilter{
		Topics: [][]*ethgo.Hash{
			{&id},
		},
	}

	filter.SetFromUint64(0)
	filter.SetToUint64(100)

	logs, err := cluster.Servers[0].JSONRPC().Eth().GetLogs(filter)
	assert.NoError(t, err)

	// Assert that all state syncs are executed successfully
	checkLogs(t, logs, num,
		func(_ int, status ResultEventStatus) bool {
			return status == ResultEventSuccess
		})
}
