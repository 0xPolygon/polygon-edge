package polybft

import (
	"github.com/0xPolygon/polygon-edge/blockchain"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/ethgo"
)

// eventsGetter is a struct for getting missed and current events
// of specified type from specified blocks
type eventsGetter[T contractsapi.EventAbi] struct {
	blockchain   blockchainBackend
	saveEventsFn func([]T) error
	parseEventFn func(*types.Header, *ethgo.Log) (T, bool, error)
	isValidLogFn func(*types.Log) bool
}

// getFromBlocks gets events of specified type from specified blocks
// and saves them using the provided saveEventsFn
func (m *eventsGetter[T]) getFromBlocks(fromBlock, toBlock uint64) error {
	var missedEvents []T

	for i := fromBlock; i <= toBlock; i++ {
		blockHeader, found := m.blockchain.GetHeaderByNumber(i)
		if !found {
			return blockchain.ErrNoBlock
		}

		receipts, err := m.blockchain.GetReceiptsByHash(blockHeader.Hash)
		if err != nil {
			return err
		}

		eventsFromBlock, err := m.getEventsFromReceipts(blockHeader, receipts)
		if err != nil {
			return err
		}

		missedEvents = append(missedEvents, eventsFromBlock...)
	}

	if len(missedEvents) > 0 {
		return m.saveEventsFn(missedEvents)
	}

	return nil
}

// getEventsFromReceipts returns events of specified type from block transaction receipts
func (m *eventsGetter[T]) getEventsFromReceipts(blockHeader *types.Header,
	receipts []*types.Receipt) ([]T, error) {
	var events []T

	for _, receipt := range receipts {
		if receipt.Status == nil || *receipt.Status != types.ReceiptSuccess {
			continue
		}

		for _, log := range receipt.Logs {
			if m.isValidLogFn != nil && !m.isValidLogFn(log) {
				continue
			}

			event, doesMatch, err := m.parseEventFn(blockHeader, convertLog(log))
			if err != nil {
				return nil, err
			}

			if !doesMatch {
				continue
			}

			events = append(events, event)
		}
	}

	return events, nil
}

// saveBlockEvents gets events of specified block from block receipts
// and saves them using the provided saveEventsFn
func (m *eventsGetter[T]) saveBlockEvents(blockHeader *types.Header,
	receipts []*types.Receipt) error {
	events, err := m.getEventsFromReceipts(blockHeader, receipts)
	if err != nil {
		return err
	}

	return m.saveEventsFn(events)
}
