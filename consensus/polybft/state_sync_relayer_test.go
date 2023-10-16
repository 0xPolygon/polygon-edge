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

func TestStateSyncRelayer_PostBlock(t *testing.T) {
	t.Parallel()

	testKey := createTestKey(t)
	successStatus := types.ReceiptSuccess
	block1Hash := types.StringToHash("x087887823ff23423")
	stateSyncAddr := types.StringToAddress("0x56563")

	result1Log := createTestLogForStateSyncResultEvent(t, 1)
	result2Log := createTestLogForStateSyncResultEvent(t, 2)
	result3Log := createTestLogForStateSyncResultEvent(t, 3)
	result4Log := createTestLogForStateSyncResultEvent(t, 4)
	commitment1Log := createTestLogForNewCommitmentEvent(t, stateSyncAddr, 2, 3, types.StringToHash("0x2"))
	commitment2Log := createTestLogForNewCommitmentEvent(t, stateSyncAddr, 4, 4, types.StringToHash("0x3"))
	receipts := [][]*types.Receipt{
		{
			{
				Status: &successStatus,
				Logs: []*types.Log{
					{}, commitment1Log, result1Log,
				},
			},
		},
		{
			{
				Status: &successStatus,
				Logs: []*types.Log{
					{}, commitment2Log,
				},
			},
		},
		{
			{
				Status: &successStatus,
				Logs: []*types.Log{
					result2Log,
				},
			},
		},
		{
			{
				Status: &successStatus,
				Logs: []*types.Log{
					result3Log,
				},
			},
		},
		{},
		{
			{
				Status: &successStatus,
				Logs: []*types.Log{
					result4Log,
				},
			},
		},
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
		state,
		proofMock,
		blockhainMock,
		testKey,
		&stateSyncRelayerConfig{
			maxAttemptsToSend:        6,
			maxBlocksToWaitForResend: 1,
		},
		hclog.Default(),
	)

	blockhainMock.On("CurrentHeader").Return(&types.Header{Number: 2}).Once()
	blockhainMock.On("CurrentHeader").Return(&types.Header{Number: 3}).Once()
	blockhainMock.On("CurrentHeader").Return(&types.Header{Number: 4}).Once()
	blockhainMock.On("CurrentHeader").Return(&types.Header{Number: 6}).Once()
	blockhainMock.On("GetReceiptsByHash", block1Hash).Return(receipts[0], nil).Once()
	blockhainMock.On("GetHeaderByNumber", uint64(1)).Return(&types.Header{Hash: block1Hash}).Once()
	dummyTxRelayer.On("SendTransaction", mock.Anything, testKey).Return((*ethgo.Receipt)(nil), nil).Times(2)
	dummyTxRelayer.On("SendTransaction", mock.Anything, testKey).Return((*ethgo.Receipt)(nil), errors.New("e")).Once()
	dummyTxRelayer.On("SendTransaction", mock.Anything, testKey).Return((*ethgo.Receipt)(nil), nil).Once()

	require.NoError(t, stateSyncRelayer.Init())

	// post first block (number 2)
	require.NoError(t, stateSyncRelayer.PostBlock(&PostBlockRequest{
		FullBlock: &types.FullBlock{
			Block: &types.Block{
				Header: &types.Header{
					Number: 2,
				},
			},
			Receipts: receipts[1],
		},
	}))

	time.Sleep(time.Second * 2) // wait for some time

	// check if everything is correct, 3 events should be in database, first one should be marked as sent
	ssrStateData, err := state.StateSyncStore.getStateSyncRelayerStateData()

	require.NoError(t, err)
	require.Equal(t, uint64(2), ssrStateData.LastBlockNumber)

	events, err := state.StateSyncStore.getAllAvailableEvents(0)

	require.NoError(t, err)
	require.Len(t, events, 3)
	require.True(t, events[0].SentStatus)
	require.False(t, events[1].SentStatus)
	require.False(t, events[2].SentStatus)

	for bn := uint64(3); bn <= uint64(6); bn++ {
		t.Logf("processing block %d", bn)

		// post second block
		require.NoError(t, stateSyncRelayer.PostBlock(&PostBlockRequest{
			FullBlock: &types.FullBlock{
				Block: &types.Block{
					Header: &types.Header{
						Number: bn,
					},
				},
				Receipts: receipts[bn-1],
			},
		}))
		time.Sleep(time.Second * 2) // wait for some time

		// check if everything is correct, 3 events should be in database, first one should be marked as sent
		ssrStateData, err := state.StateSyncStore.getStateSyncRelayerStateData()

		require.NoError(t, err)
		require.Equal(t, bn, ssrStateData.LastBlockNumber)

		events, err := state.StateSyncStore.getAllAvailableEvents(0)

		require.NoError(t, err)

		if bn < 5 { // on block 5 sending fails
			require.Len(t, events, 5-int(bn))
		} else {
			require.Len(t, events, 6-int(bn))
		}

		for i, e := range events {
			require.Equal(t, i == 0, e.SentStatus)
		}
	}

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
