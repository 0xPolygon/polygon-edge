package polybft

import (
	"sync"

	"github.com/0xPolygon/polygon-edge/blockchain"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/ethgo"
)

// EventSubscriber specifies functions needed for a component to subscribe to eventProvider
type EventSubscriber interface {
	GetLogFilters() map[types.Address][]types.Hash
	AddLog(header *types.Header, log *ethgo.Log, dbTx DBTransaction) error
}

// EventProvider represents an event provider in a blockchain system
// that returns desired events to subscribers from finalized blocks
// Note that this provider needs to be called manually on each block finalization.
// The EventProvider struct has the following fields:
// - blockchain: A blockchainBackend type that represents the blockchain backend used by the event provider.
// - subscribers: A map[string]eventSubscriber that stores the subscribers of the event provider.
// - allFilters: A map[types.Address]map[types.Hash][]string that stores the filters for event logs.
type EventProvider struct {
	lock                sync.Mutex
	subscriberIDCounter uint64

	blockchain blockchainBackend

	subscribers map[uint64]EventSubscriber
	allFilters  map[types.Address]map[types.Hash][]uint64
}

// NewEventProvider returns a new instance of eventProvider
func NewEventProvider(blockchain blockchainBackend) *EventProvider {
	return &EventProvider{
		blockchain:  blockchain,
		subscribers: make(map[uint64]EventSubscriber, 0),
		allFilters:  make(map[types.Address]map[types.Hash][]uint64, 0),
	}
}

// Subscribe subscribes given EventSubscriber to desired logs (events)
func (e *EventProvider) Subscribe(subscriber EventSubscriber) {
	e.lock.Lock()
	e.subscriberIDCounter++
	subscriberID := e.subscriberIDCounter
	e.subscribers[e.subscriberIDCounter] = subscriber
	e.lock.Unlock()

	for address, filters := range subscriber.GetLogFilters() {
		existingAddressFilters, exist := e.allFilters[address]
		if !exist {
			existingAddressFilters = make(map[types.Hash][]uint64, 0)
			e.allFilters[address] = existingAddressFilters
		}

		for _, f := range filters {
			existingAddressFilters[f] = append(existingAddressFilters[f], subscriberID)
		}
	}
}

// GetEventsFromBlocks gets all desired logs (events) for each subscriber in given block range
//
// Inputs:
// - lastProcessedBlock - last finalized block that was processed for desired events
// - latestBlock - latest finalized block
//
// Returns:
// - nil - if getting events finished successfully
// - error - if a block or its receipts could not be retrieved from blockchain
func (e *EventProvider) GetEventsFromBlocks(lastProcessedBlock uint64,
	latestBlock *types.FullBlock,
	dbTx DBTransaction) error {
	if err := e.getEventsFromBlocks(lastProcessedBlock+1, latestBlock.Block.Number()-1, dbTx); err != nil {
		return err
	}

	return e.getEventsFromReceipts(latestBlock.Block.Header, latestBlock.Receipts, dbTx)
}

// getEventsFromBlocks gets all desired logs (events) for each subscriber in given block range
//
// Inputs:
// - from - block from which log (event) retrieval should start
// - to - last block from which log (event) retrieval should be done
//
// Returns:
// - nil - if getting events finished successfully
// - error - if a block or its receipts could not be retrieved from blockchain
func (e *EventProvider) getEventsFromBlocks(from, to uint64, dbTx DBTransaction) error {
	for i := from; i <= to; i++ {
		blockHeader, found := e.blockchain.GetHeaderByNumber(i)
		if !found {
			return blockchain.ErrNoBlock
		}

		receipts, err := e.blockchain.GetReceiptsByHash(blockHeader.Hash)
		if err != nil {
			return err
		}

		if err := e.getEventsFromReceipts(blockHeader, receipts, dbTx); err != nil {
			return err
		}
	}

	return nil
}

// getEventsFromReceipts gets all desired logs (events) for each subscriber from given block receipts
//
// Inputs:
// - blockHeader - header of block from whose receipts the function will retrieve logs (events)
// - receipts - given block receipts from which the function will retrieve logs (events)
//
// Returns:
// - nil - if getting events finished successfully
// - error - if a subscriber for a certain log (event) returns an error on log (event) handling
func (e *EventProvider) getEventsFromReceipts(blockHeader *types.Header,
	receipts []*types.Receipt,
	dbTx DBTransaction) error {
	for _, receipt := range receipts {
		if receipt.Status == nil || *receipt.Status != types.ReceiptSuccess {
			continue
		}

		for _, log := range receipt.Logs {
			logFilters, isRelevantLog := e.allFilters[log.Address]
			if !isRelevantLog {
				continue
			}

			for logFilter, subscribers := range logFilters {
				if log.Topics[0] == logFilter {
					convertedLog := convertLog(log)
					for _, subscriber := range subscribers {
						if err := e.subscribers[subscriber].AddLog(blockHeader, convertedLog, dbTx); err != nil {
							return err
						}
					}
				}
			}
		}
	}

	return nil
}

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
	allEvents, err := e.getEventsFromBlocks(lastProcessedBlock+1, currentBlock.Block.Number()-1)
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

// getEventsFromBlocks gets events of specified type from all the blocks specified [from, to]
func (e *eventsGetter[T]) getEventsFromBlocks(from, to uint64) ([]T, error) {
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
