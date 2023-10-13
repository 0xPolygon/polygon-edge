package polybft

import (
	"testing"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
)

func TestEventDBInsertRetry_GetEvents(t *testing.T) {
	receipt := &types.Receipt{
		Logs: []*types.Log{
			createTestLogForTransferEvent(t, contracts.ValidatorSetContract, types.ZeroAddress, types.ZeroAddress, 10),
		},
	}
	receipt.SetStatus(types.ReceiptSuccess)

	backend := new(blockchainMock)
	backend.On("GetHeaderByNumber", mock.Anything).Return(&types.Header{
		Hash: types.BytesToHash([]byte{0, 1, 2, 3}),
	}, true)
	backend.On("GetReceiptsByHash", mock.Anything).Return([]*types.Receipt{receipt}, nil)

	retryManager := &eventsGetter[*contractsapi.TransferEvent]{
		receiptsGetter: receiptsGetter{
			blockchain: backend,
		},
		isValidLogFn: func(l *types.Log) bool {
			return l.Address == contracts.ValidatorSetContract
		},
		parseEventFn: func(h *types.Header, l *ethgo.Log) (*contractsapi.TransferEvent, bool, error) {
			var e contractsapi.TransferEvent
			doesMatch, err := e.ParseLog(l)

			return &e, doesMatch, err
		},
	}

	events, err := retryManager.getFromBlocks(0, &types.FullBlock{
		Block:    &types.Block{Header: &types.Header{Number: 2}},
		Receipts: []*types.Receipt{},
	})

	require.NoError(t, err)
	require.Len(t, events, 1)
}
