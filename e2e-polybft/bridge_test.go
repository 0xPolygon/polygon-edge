package e2e

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
)

var stateSyncResultEvent = abi.MustNewEvent(`event StateSyncResult(
		uint256 indexed counter,
		uint8 indexed status,
		bytes32 message)`)

// event StateSyncResult(uint256 indexed counter, ResultStatus indexed status, bytes32 message);

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
	assert.Len(t, logs, expectedCount)
	for i, log := range logs {
		res, err := stateSyncResultEvent.ParseLog(log)
		assert.NoError(t, err)
		t.Logf("Block Number=%d, Decoded Log=%v", log.BlockNumber, res)
		assert.True(t, assertFn(i, ResultEventStatus(res["status"].(uint8))))
	}
}

func TestE2E_Bridge_MainWorkflow(t *testing.T) {
	cluster := framework.NewTestCluster(t, 5) // , framework.WithBridge())
	defer cluster.Stop()

	// wait for a couple of blocks
	require.NoError(t, cluster.WaitForBlock(2, 1*time.Minute))

	// send a few transactions to the bridge
	num := 10
	var wallets, amounts []string
	for i := 0; i < num; i++ {
		wallets = append(wallets, fmt.Sprintf("0x%040x", 1))
		amounts = append(amounts, fmt.Sprintf("%d", 100))
	}

	require.NoError(
		t,
		cluster.EmitTransfer(
			contracts.NativeTokenContract.String(),
			strings.Join(wallets, ","),
			strings.Join(amounts, ","),
		),
	)

	// wait for a few more sprints
	require.NoError(t, cluster.WaitForBlock(30, 2*time.Minute))

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
