package polybft

import (
	"math/big"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/common"
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
	block2Hash := types.StringToHash("x08ffeee23ff23423")
	stateSyncAddr := types.StringToAddress("0x56563")
	commitment1Log := createTestLogForNewCommitmentEvent(t, stateSyncAddr, 1, 2, types.StringToHash("0x1"))
	commitment2Log := createTestLogForNewCommitmentEvent(t, stateSyncAddr, 3, 3, types.StringToHash("0x2"))
	commitment3Log := createTestLogForNewCommitmentEvent(t, stateSyncAddr, 4, 6, types.StringToHash("0x3"))
	commitment4Log := createTestLogForNewCommitmentEvent(t, stateSyncAddr, 7, 7, types.StringToHash("0x4"))
	receiptsBlock1 := []*types.Receipt{
		{
			Status: &successStatus,
			Logs: []*types.Log{
				{}, commitment1Log,
			},
		},
	}
	receiptsBlock2 := []*types.Receipt{
		{
			Status: &successStatus,
			Logs: []*types.Log{
				{}, commitment2Log,
			},
		},
		{
			Status: &successStatus,
			Logs: []*types.Log{
				commitment3Log,
			},
		},
	}
	receiptsBlock3 := []*types.Receipt{
		{
			Status: &successStatus,
			Logs: []*types.Log{
				{}, commitment4Log,
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
		hclog.Default(),
	)

	// post block 2, last state sync fails
	blockhainMock.On("GetHeaderByNumber", uint64(1)).Return(&types.Header{
		Hash: block1Hash,
	}).Once()
	blockhainMock.On("GetReceiptsByHash", block1Hash).Return(receiptsBlock1, nil).Once()
	// fail on stateSyncID == 6 -> last one in first try
	dummyTxRelayer.On("SendTransaction", mock.Anything, testKey).Return(
		&ethgo.Receipt{Status: uint64(types.ReceiptSuccess)}, nil).Times(5)
	dummyTxRelayer.On("SendTransaction", mock.Anything, testKey).Return(
		&ethgo.Receipt{Status: uint64(types.ReceiptFailed)}, nil).Once()

	require.NoError(t, stateSyncRelayer.Init())

	// post first block
	require.NoError(t, stateSyncRelayer.PostBlock(&common.PostBlockRequest{
		FullBlock: &types.FullBlock{
			Block: &types.Block{
				Header: &types.Header{
					Number: 2,
				},
			},
			Receipts: receiptsBlock2,
		},
	}))

	time.Sleep(time.Second * 2) // wait for some time

	// we need to be sure that 5 events are processed and last one failed
	blockNumber, eventID, err := state.StateSyncStore.getStateSyncRelayerData()

	blockhainMock.AssertExpectations(t)
	require.NoError(t, err)
	require.Equal(t, uint64(2), blockNumber)
	require.Equal(t, uint64(6), eventID)

	// post block 3, all the events should be processed and everything should pass
	blockhainMock.On("GetHeaderByNumber", uint64(2)).Return(&types.Header{
		Hash: block2Hash,
	}).Once()
	blockhainMock.On("GetReceiptsByHash", block2Hash).Return(receiptsBlock2, nil).Once()
	dummyTxRelayer.On("SendTransaction", mock.Anything, testKey).Return(
		&ethgo.Receipt{Status: uint64(types.ReceiptSuccess)}, nil).Times(2)

	// post another block
	require.NoError(t, stateSyncRelayer.PostBlock(&common.PostBlockRequest{
		FullBlock: &types.FullBlock{
			Block: &types.Block{
				Header: &types.Header{
					Number: 3,
				},
			},
			Receipts: receiptsBlock3,
		},
	}))

	stateSyncRelayer.Close()

	time.Sleep(time.Second * 2) // wait for some time

	// we need to be sure that 5 events are processed and last one failed
	blockNumber, eventID, err = state.StateSyncStore.getStateSyncRelayerData()

	blockhainMock.AssertExpectations(t)
	require.NoError(t, err)
	require.Equal(t, uint64(4), blockNumber)
	require.Equal(t, uint64(8), eventID)
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

	topics := []types.Hash{
		types.Hash(evnt.Sig()),
		types.BytesToHash(encodedData1),
		types.BytesToHash(encodedData2),
	}

	return &types.Log{
		Address: stateSyncAddr,
		Topics:  topics,
		Data:    rootHash[:],
	}
}
