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
	// blockchain is an abstraction of blockchain that provides necessary functions
	// for querying blockchain data (blocks, receipts, etc.)
	blockchain blockchainBackend
	// parseEventFn is a plugin function used to parse the event from transaction log
	parseEventFn func(*types.Header, *ethgo.Log) (T, bool, error)
	// isValidLogFn is a plugin function that validates the log
	// for example: if it was sent from the desired address
	isValidLogFn func(*types.Log) bool
}

// getFromBlocks gets events of specified type from specified blocks
// and saves them using the provided saveEventsFn
func (e *eventsGetter[T]) getFromBlocks(lastProcessedBlock uint64,
	currentBlock *types.FullBlock) ([]T, error) {
	allEvents, err := e.getEventsFromAllBlocks(lastProcessedBlock+1, currentBlock.Block.Number()-1)
	if err != nil {
		return nil, err
	}

	currentEvents, err := e.getEventsFromReceipts(currentBlock.Block.Header, currentBlock.Receipts)
	if err != nil {
		return nil, err
	}

	allEvents = append(allEvents, currentEvents...)

	return allEvents, nil
}

// getEventsFromAllBlocks gets events of specified type from all the blocks specified [from, to]
func (e *eventsGetter[T]) getEventsFromAllBlocks(from, to uint64) ([]T, error) {
	var allEvents []T

	for i := from; i <= to; i++ {
		blockHeader, found := e.blockchain.GetHeaderByNumber(i)
		if !found {
			return nil, blockchain.ErrNoBlock
		}

		receipts, err := e.blockchain.GetReceiptsByHash(blockHeader.Hash)
		if err != nil {
			return nil, err
		}

		eventsFromBlock, err := e.getEventsFromReceipts(blockHeader, receipts)
		if err != nil {
			return nil, err
		}

		allEvents = append(allEvents, eventsFromBlock...)
	}

	return allEvents, nil
}

// getEventsFromReceipts returns events of specified type from block transaction receipts
func (e *eventsGetter[T]) getEventsFromReceipts(blockHeader *types.Header,
	receipts []*types.Receipt) ([]T, error) {
	var events []T

	for _, receipt := range receipts {
		if receipt.Status == nil || *receipt.Status != types.ReceiptSuccess {
			continue
		}

		for _, log := range receipt.Logs {
			if e.isValidLogFn != nil && !e.isValidLogFn(log) {
				continue
			}

			event, doesMatch, err := e.parseEventFn(blockHeader, convertLog(log))
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
