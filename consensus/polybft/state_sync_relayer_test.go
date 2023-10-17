package polybft

import (
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
)

func TestStateSyncRelayer_FullWorkflow(t *testing.T) {
	t.Parallel()

	testKey := createTestKey(t)
	stateSyncAddr := types.StringToAddress("0x56563")

	resultLogs := []*types.Log{
		createTestLogForStateSyncResultEvent(t, 1), createTestLogForStateSyncResultEvent(t, 2),
		createTestLogForStateSyncResultEvent(t, 3), createTestLogForStateSyncResultEvent(t, 4),
		createTestLogForStateSyncResultEvent(t, 5),
	}
	commitmentLogs := []*types.Log{
		createTestLogForNewCommitmentEvent(t, stateSyncAddr, 1, 1, types.StringToHash("0x2")),
		createTestLogForNewCommitmentEvent(t, stateSyncAddr, 2, 3, types.StringToHash("0x2")),
		createTestLogForNewCommitmentEvent(t, stateSyncAddr, 4, 5, types.StringToHash("0x2")),
	}

	headers := []*types.Header{
		{Number: 2}, {Number: 3}, {Number: 4}, {Number: 5}, {Number: 5},
	}

	proofMock := &mockStateSyncProofRetriever{
		fn: func(stateSyncID uint64) (types.Proof, error) {
			return types.Proof{
				Data: []types.Hash{types.StringToHash("0x1122334455")},
				Metadata: map[string]interface{}{
					"StateSync": map[string]interface{}{
						"ID":       stateSyncID,
						"Sender":   types.StringToAddress("0xffee"),
						"Receiver": types.StringToAddress("0xeeff"),
						"Data":     nil,
					},
				},
			}, nil
		},
	}
	blockhainMock := &blockchainMock{}
	dummyTxRelayer := newDummyStakeTxRelayer(t, nil)
	state := newTestState(t)

	stateSyncRelayer := NewStateSyncRelayer(
		dummyTxRelayer,
		stateSyncAddr,
		state.StateSyncStore,
		proofMock,
		blockhainMock,
		testKey,
		&stateSyncRelayerConfig{
			maxAttemptsToSend:        6,
			maxBlocksToWaitForResend: 1,
			maxEventsPerBatch:        1,
		},
		hclog.Default(),
	)

	for _, h := range headers {
		blockhainMock.On("CurrentHeader").Return(h).Once()
	}

	// send first two events without errors
	dummyTxRelayer.On("SendTransaction", mock.Anything, testKey).Return((*ethgo.Receipt)(nil), nil).Times(2)
	// fail 3rd time
	dummyTxRelayer.On("SendTransaction", mock.Anything, testKey).Return(
		(*ethgo.Receipt)(nil), errors.New("e")).Once()
	// send 3 events all at once at the end
	dummyTxRelayer.On("SendTransaction", mock.Anything, testKey).Return((*ethgo.Receipt)(nil), nil).Once()

	require.NoError(t, stateSyncRelayer.Init())

	// post 1st block
	require.NoError(t, stateSyncRelayer.ProcessLog(headers[0], convertLog(commitmentLogs[0]), nil))
	require.NoError(t, stateSyncRelayer.ProcessLog(headers[0], convertLog(commitmentLogs[1]), nil))
	require.NoError(t, stateSyncRelayer.PostBlock(&PostBlockRequest{}))

	time.Sleep(time.Second * 2) // wait for some time

	events, err := state.StateSyncStore.getAllAvailableEvents(0)

	require.NoError(t, err)
	require.Len(t, events, 3)
	require.Equal(t, uint64(1), events[0].EventID)
	require.True(t, events[0].SentStatus)
	require.False(t, events[1].SentStatus)
	require.False(t, events[2].SentStatus)

	// post 2nd block
	require.NoError(t, stateSyncRelayer.ProcessLog(headers[1], convertLog(resultLogs[0]), nil))
	require.NoError(t, stateSyncRelayer.ProcessLog(headers[1], convertLog(commitmentLogs[2]), nil))
	require.NoError(t, stateSyncRelayer.PostBlock(&PostBlockRequest{}))

	time.Sleep(time.Second * 2) // wait for some time

	events, err = state.StateSyncStore.getAllAvailableEvents(0)

	require.NoError(t, err)
	require.Len(t, events, 4)
	require.True(t, events[0].SentStatus)
	require.Equal(t, uint64(2), events[0].EventID)
	require.False(t, events[1].SentStatus)
	require.False(t, events[2].SentStatus)

	// post 3rd block
	require.NoError(t, stateSyncRelayer.ProcessLog(headers[2], convertLog(resultLogs[1]), nil))
	require.NoError(t, stateSyncRelayer.PostBlock(&PostBlockRequest{}))

	time.Sleep(time.Second * 2) // wait for some time

	events, err = state.StateSyncStore.getAllAvailableEvents(0)

	require.NoError(t, err)
	require.Len(t, events, 3)
	require.True(t, events[0].SentStatus)
	require.Equal(t, uint64(3), events[0].EventID)
	require.False(t, events[1].SentStatus)

	// post 4th block - will not provide result, so one more SendTransaction will be triggered
	stateSyncRelayer.config.maxEventsPerBatch = 3 // send all 3 left events at once

	require.NoError(t, stateSyncRelayer.PostBlock(&PostBlockRequest{}))

	time.Sleep(time.Second * 2) // wait for some time

	events, err = state.StateSyncStore.getAllAvailableEvents(0)

	require.NoError(t, err)
	require.Len(t, events, 3)
	require.True(t, events[0].SentStatus && events[1].SentStatus && events[2].SentStatus)

	// post 5th block
	require.NoError(t, stateSyncRelayer.ProcessLog(headers[4], convertLog(resultLogs[2]), nil))
	require.NoError(t, stateSyncRelayer.ProcessLog(headers[4], convertLog(resultLogs[3]), nil))
	require.NoError(t, stateSyncRelayer.ProcessLog(headers[4], convertLog(resultLogs[4]), nil))
	require.NoError(t, stateSyncRelayer.PostBlock(&PostBlockRequest{}))

	time.Sleep(time.Second * 2) // wait for some time

	events, err = state.StateSyncStore.getAllAvailableEvents(0)

	require.NoError(t, err)
	require.Len(t, events, 0)

	stateSyncRelayer.Close()
	time.Sleep(time.Second)

	blockhainMock.AssertExpectations(t)
	dummyTxRelayer.AssertExpectations(t)
}

type mockStateSyncProofRetriever struct {
	fn func(uint64) (types.Proof, error)
}

func (m *mockStateSyncProofRetriever) GetStateSyncProof(stateSyncID uint64) (types.Proof, error) {
	if m.fn != nil {
		return m.fn(stateSyncID)
	}

	return types.Proof{}, nil
}

func createTestLogForNewCommitmentEvent(
	t *testing.T,
	stateSyncAddr types.Address,
	startEventID uint64,
	endEventID uint64,
	rootHash types.Hash) *types.Log {
	t.Helper()

	var evnt contractsapi.NewCommitmentEvent

	encodedData1, err := abi.MustNewType("uint256").Encode(new(big.Int).SetUint64(startEventID))
	require.NoError(t, err)

	encodedData2, err := abi.MustNewType("uint256").Encode(new(big.Int).SetUint64(endEventID))
	require.NoError(t, err)

	return &types.Log{
		Address: stateSyncAddr,
		Topics: []types.Hash{
			types.Hash(evnt.Sig()),
			types.BytesToHash(encodedData1),
			types.BytesToHash(encodedData2),
		},
		Data: rootHash[:],
	}
}
