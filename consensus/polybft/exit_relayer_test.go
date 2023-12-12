package polybft

import (
	"encoding/hex"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
)

func TestExitRelayer_FullWorkflow(t *testing.T) {
	t.Parallel()

	testKey := createTestKey(t)
	exitHelperAddr := types.StringToAddress("0xExitHelper")

	resultLogs := []*types.Log{
		createTestLogForExitProcessedEvent(t, 1, exitHelperAddr), createTestLogForExitProcessedEvent(t, 2, exitHelperAddr),
		createTestLogForExitProcessedEvent(t, 3, exitHelperAddr), createTestLogForExitProcessedEvent(t, 4, exitHelperAddr),
	}
	checkpointSubmittedLogs := []*types.Log{
		createTestLogForCheckpointSubmittedEvent(t, exitHelperAddr, 1, 10, types.StringToHash("0x2")),
		createTestLogForCheckpointSubmittedEvent(t, exitHelperAddr, 2, 20, types.StringToHash("0x2")),
	}

	blockhainMock := &blockchainMock{}
	dummyTxRelayer := newDummyStakeTxRelayer(t, nil)
	state := newTestState(t)

	insertTestExitEvent := func(exitEventID, epoch, block uint64) {
		require.NoError(t, state.ExitStore.insertExitEvent(
			&ExitEvent{
				L2StateSyncedEvent: &contractsapi.L2StateSyncedEvent{
					ID:       big.NewInt(int64(exitEventID)),
					Sender:   types.StringToAddress("0xffee"),
					Receiver: types.StringToAddress("0xeeff"),
				},
				EpochNumber: epoch,
				BlockNumber: block,
			},
			nil,
		))
	}

	// insert exit events for epoch 1
	insertTestExitEvent(1, 1, 10)
	insertTestExitEvent(2, 1, 10)

	// insert exit events for epoch 2
	insertTestExitEvent(3, 2, 20)
	insertTestExitEvent(4, 2, 20)

	headers := []*types.Header{
		{Number: 11}, {Number: 12}, {Number: 13},
	}

	for _, h := range headers {
		blockhainMock.On("CurrentHeader").Return(h).Once()
	}

	proofMock := &mockExitProofRetriever{
		fn: func(exitEventID uint64) (types.Proof, error) {
			mockEvent := &contractsapi.L2StateSyncedEvent{
				ID:       big.NewInt(int64(exitEventID)),
				Sender:   types.StringToAddress("0xffee"),
				Receiver: types.StringToAddress("0xeeff"),
			}
			encodedEvent, err := mockEvent.Encode()
			require.NoError(t, err)

			return types.Proof{
				Data: []types.Hash{types.StringToHash("0x1122334455")},
				Metadata: map[string]interface{}{
					"ExitEvent":       hex.EncodeToString(encodedEvent),
					"CheckpointBlock": big.NewInt(10),
					"LeafIndex":       uint64(0),
				},
			}, nil
		},
	}

	exitRelayer := newExitRelayer(
		dummyTxRelayer,
		testKey,
		proofMock,
		blockhainMock,
		state.ExitStore,
		&relayerConfig{
			maxAttemptsToSend:        6,
			maxBlocksToWaitForResend: 1,
			maxEventsPerBatch:        2,
			eventExecutionAddr:       exitHelperAddr,
		},
		hclog.Default(),
	)

	// send first two events without errors
	dummyTxRelayer.On("SendTransaction", mock.Anything, testKey).Return((*ethgo.Receipt)(nil), nil).Once()
	// fail 3rd time
	dummyTxRelayer.On("SendTransaction", mock.Anything, testKey).Return(
		(*ethgo.Receipt)(nil), errors.New("e")).Once()
	// send 2 events all at once at the end
	dummyTxRelayer.On("SendTransaction", mock.Anything, testKey).Return((*ethgo.Receipt)(nil), nil).Once()

	require.NoError(t, exitRelayer.Init())

	// post 1st block
	require.NoError(t, exitRelayer.AddLog(convertLog(checkpointSubmittedLogs[0])))
	require.NoError(t, exitRelayer.AddLog(convertLog(checkpointSubmittedLogs[1])))
	require.NoError(t, exitRelayer.PostBlock(&PostBlockRequest{}))

	time.Sleep(time.Second * 2) // wait for some time

	events, err := state.ExitStore.GetAllAvailableRelayerEvents(0)

	require.NoError(t, err)
	require.Len(t, events, 4)
	require.Equal(t, uint64(1), events[0].EventID)
	// first two events should be executed
	require.True(t, events[0].SentStatus)
	require.True(t, events[1].SentStatus)
	require.False(t, events[2].SentStatus)
	require.False(t, events[3].SentStatus)

	// post 2nd block
	// send exit processed events for the two executed events
	require.NoError(t, exitRelayer.AddLog(convertLog(resultLogs[0])))
	require.NoError(t, exitRelayer.AddLog(convertLog(resultLogs[1])))
	require.NoError(t, exitRelayer.PostBlock(&PostBlockRequest{}))

	time.Sleep(time.Second * 2) // wait for some time

	events, err = state.ExitStore.GetAllAvailableRelayerEvents(0)

	require.NoError(t, err)
	// should only have two since first two were successfully executed
	require.Len(t, events, 2)
	require.True(t, events[0].SentStatus)
	require.True(t, events[1].SentStatus)
	require.Equal(t, uint64(3), events[0].EventID)
	require.Equal(t, uint64(4), events[1].EventID)

	// since sending of second batch failed
	// we do post 3rd block to see if they are sent in this one
	require.NoError(t, exitRelayer.PostBlock(&PostBlockRequest{}))

	time.Sleep(time.Second * 2) // wait for some time

	events, err = state.ExitStore.GetAllAvailableRelayerEvents(0)

	require.NoError(t, err)
	// should only have two since first two were successfully executed
	require.Len(t, events, 2)
	require.True(t, events[0].SentStatus)
	require.True(t, events[1].SentStatus)
	require.Equal(t, uint64(3), events[0].EventID)
	require.Equal(t, uint64(4), events[1].EventID)

	// send exit processed events for the two executed events
	require.NoError(t, exitRelayer.AddLog(convertLog(resultLogs[2])))
	require.NoError(t, exitRelayer.AddLog(convertLog(resultLogs[3])))

	time.Sleep(time.Second * 2) // wait for some time

	events, err = state.ExitStore.GetAllAvailableRelayerEvents(0)

	require.NoError(t, err)
	// should have no events since all of them were executed successfully
	require.Len(t, events, 0)

	exitRelayer.Close()
	time.Sleep(time.Second)

	blockhainMock.AssertExpectations(t)
	dummyTxRelayer.AssertExpectations(t)
}

type mockExitProofRetriever struct {
	fn func(uint64) (types.Proof, error)
}

func (m *mockExitProofRetriever) GenerateExitProof(exitEventID uint64) (types.Proof, error) {
	if m.fn != nil {
		return m.fn(exitEventID)
	}

	return types.Proof{}, nil
}

func createTestLogForExitProcessedEvent(t *testing.T, exitEventID uint64, exitHelperAddr types.Address) *types.Log {
	t.Helper()

	topics := make([]types.Hash, 3)
	topics[0] = types.Hash(new(contractsapi.ExitProcessedEvent).Sig())
	topics[1] = types.BytesToHash(common.EncodeUint64ToBytes(exitEventID))
	topics[2] = types.BytesToHash(common.EncodeUint64ToBytes(1)) // Success = true
	someType := abi.MustNewType("tuple(string field1, string field2)")
	encodedData, err := someType.Encode(map[string]string{"field1": "value1", "field2": "value2"})
	require.NoError(t, err)

	return &types.Log{
		Address: exitHelperAddr,
		Topics:  topics,
		Data:    encodedData,
	}
}

func createTestLogForCheckpointSubmittedEvent(
	t *testing.T,
	exitHelperAddr types.Address,
	epoch uint64,
	checkpointBlockNumber uint64,
	exitRootHash types.Hash) *types.Log {
	t.Helper()

	topics := make([]types.Hash, 3)
	topics[0] = types.Hash(new(contractsapi.CheckpointSubmittedEvent).Sig())
	topics[1] = types.BytesToHash(common.EncodeUint64ToBytes(epoch))
	topics[2] = types.BytesToHash(common.EncodeUint64ToBytes(checkpointBlockNumber))

	return &types.Log{
		Address: exitHelperAddr,
		Topics:  topics,
		Data:    exitRootHash[:],
	}
}
