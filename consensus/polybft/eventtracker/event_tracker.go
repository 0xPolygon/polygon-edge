package eventtracker

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/0xPolygon/polygon-edge/helper/common"
	hcf "github.com/hashicorp/go-hclog"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/blocktracker"
)

// EventSubscriber is an interface that defines methods for handling tracked logs (events) from a blockchain
type EventSubscriber interface {
	AddLog(log *ethgo.Log) error
}

// BlockProvider is an interface that defines methods for retrieving blocks and logs from a blockchain
type BlockProvider interface {
	GetBlockByHash(hash ethgo.Hash, full bool) (*ethgo.Block, error)
	GetBlockByNumber(i ethgo.BlockNumber, full bool) (*ethgo.Block, error)
	GetLogs(filter *ethgo.LogFilter) ([]*ethgo.Log, error)
}

// EventTrackerConfig is a struct that holds configuration of a EventTracker
type EventTrackerConfig struct {
	// RPCEndpoint is the full json rpc url on some node on a tracked chain
	RPCEndpoint string

	// StartBlockFromConfig represents a starting block from which tracker starts to
	// track events from a tracked chain.
	// This is only relevant on the first start of the tracker. After it processes blocks,
	// it will start from the last processed block, and not from the StartBlockFromConfig.
	StartBlockFromConfig uint64

	// NumBlockConfirmations defines how many blocks must pass from a certain block,
	// to consider that block as final on the tracked chain.
	// This is very important for reorgs, and events from the given block will only be
	// processed if it hits this confirmation mark.
	// (e.g., NumBlockConfirmations = 3, and if the last tracked block is 10,
	// events from block 10, will only be processed when we get block 13 from the tracked chain)
	NumBlockConfirmations uint64

	// SyncBatchSize defines a batch size of blocks that will be gotten from tracked chain,
	// when tracker is out of sync and needs to sync a number of blocks.
	// (e.g., SyncBatchSize = 10, trackers last processed block is 10, latest block on tracked chain is 100,
	// it will get blocks 11-20, get logs from confirmed blocks of given batch, remove processed confirm logs
	// from memory, and continue to the next batch)
	SyncBatchSize uint64

	// MaxBacklogSize defines how many blocks we will sync up from the latest block on tracked chain.
	// If a node that has tracker, was offline for days, months, a year, it will miss a lot of blocks.
	// In the meantime, we expect the rest of nodes to have collected the desired events and did their
	// logic with them, continuing consensus and relayer stuff.
	// In order to not waste too much unnecessary time in syncing all those blocks, with MaxBacklogSize,
	// we tell the tracker to sync only latestBlock.Number - MaxBacklogSize number of blocks.
	MaxBacklogSize uint64

	// PollInterval defines a time interval in which tracker polls json rpc node
	// for latest block on the tracked chain.
	PollInterval time.Duration

	// Logger is the logger instance for event tracker
	Logger hcf.Logger

	// LogFilter defines which events are tracked and from which contracts on the tracked chain
	LogFilter map[ethgo.Address][]ethgo.Hash

	// Store is the store implementation for data that tracker saves (lastProcessedBlock and logs)
	Store EventTrackerStore

	// BlockProvider is the implementation of a provider that returns blocks and logs from tracked chain
	BlockProvider BlockProvider

	// EventSubscriber is the subscriber that requires events tracked by the event tracker
	EventSubscriber EventSubscriber
}

// EventTracker represents a tracker for events on desired contracts on some chain
type EventTracker struct {
	config *EventTrackerConfig

	closeCh chan struct{}
	once    sync.Once

	blockTracker   blocktracker.BlockTrackerInterface
	blockContainer *TrackerBlockContainer
}

// NewEventTracker is a constructor function that creates a new instance of the EventTracker struct.
//
// Example Usage:
//
//	config := &EventTracker{
//		RpcEndpoint:           "http://some-json-rpc-url.com",
//		StartBlockFromConfig:  100_000,
//		NumBlockConfirmations: 10,
//		SyncBatchSize:         20,
//		MaxBacklogSize:        10_000,
//		PollInterval:          2 * time.Second,
//		Logger:                logger,
//		Store:                 store,
//		EventSubscriber:       subscriber,
//		Provider:              provider,
//		LogFilter: TrackerLogFilter{
//			Addresses: []ethgo.Address{addressOfSomeContract},
//			IDs:       []ethgo.Hash{idHashOfSomeEvent},
//		},
//	}
//		t := NewEventTracker(config)
//
// Inputs:
//   - config (TrackerConfig): configuration of EventTracker.
//
// Outputs:
//   - A new instance of the EventTracker struct.
func NewEventTracker(config *EventTrackerConfig) (*EventTracker, error) {
	lastProcessedBlock, err := config.Store.GetLastProcessedBlock()
	if err != nil {
		return nil, err
	}

	var definiteLastProcessedBlock uint64
	if config.StartBlockFromConfig > 0 {
		definiteLastProcessedBlock = config.StartBlockFromConfig - 1
	}

	if lastProcessedBlock > definiteLastProcessedBlock {
		definiteLastProcessedBlock = lastProcessedBlock
	}

	return &EventTracker{
		config:         config,
		closeCh:        make(chan struct{}),
		blockTracker:   blocktracker.NewJSONBlockTracker(config.BlockProvider),
		blockContainer: NewTrackerBlockContainer(definiteLastProcessedBlock),
	}, nil
}

// Close closes the EventTracker by closing the closeCh channel.
// This method is used to signal the goroutines to stop.
//
// Example Usage:
//
//	tracker := NewEventTracker(config)
//	tracker.Start()
//	defer tracker.Close()
//
// Inputs: None
//
// Flow:
//  1. The Close() method is called on an instance of EventTracker.
//  2. The closeCh channel is closed, which signals the goroutines to stop.
//
// Outputs: None
func (e *EventTracker) Close() {
	close(e.closeCh)
}

// Start is a method in the EventTracker struct that starts the tracking of blocks
// and retrieval of logs from given blocks from the tracked chain.
// If the tracker was turned off (node was down) for some time, it will sync up all the missed
// blocks and logs from the last start (in regards to MaxBacklogSize field in config).
//
// Returns:
//   - nil if start passes successfully.
//   - An error if there is an error on startup of blocks tracking on tracked chain.
func (e *EventTracker) Start() error {
	e.config.Logger.Info("Starting event tracker",
		"jsonRpcEndpoint", e.config.RPCEndpoint,
		"startBlockFromConfig", e.config.StartBlockFromConfig,
		"numBlockConfirmations", e.config.NumBlockConfirmations,
		"pollInterval", e.config.PollInterval,
		"syncBatchSize", e.config.SyncBatchSize,
		"maxBacklogSize", e.config.MaxBacklogSize,
		"logFilter", e.config.LogFilter,
	)

	ctx, cancelFn := context.WithCancel(context.Background())
	go func() {
		<-e.closeCh
		cancelFn()
	}()

	go common.RetryForever(ctx, time.Second, func(context.Context) error {
		// sync up all missed blocks on start if it is not already sync up
		if err := e.syncOnStart(); err != nil {
			e.config.Logger.Error("Syncing up on start failed.", "err", err)

			return err
		}

		// start the polling of blocks
		err := e.blockTracker.Track(ctx, func(block *ethgo.Block) error {
			return e.trackBlock(block)
		})

		if common.IsContextDone(err) {
			return nil
		}

		return err
	})

	return nil
}

// trackBlock is a method in the EventTracker struct that is responsible for tracking blocks and processing their logs
//
// Inputs:
// - block: An instance of the ethgo.Block struct representing a block to track.
//
// Returns:
//   - nil if tracking block passes successfully.
//   - An error if there is an error on tracking given block.
func (e *EventTracker) trackBlock(block *ethgo.Block) error {
	if !e.blockContainer.IsOutOfSync(block) {
		e.blockContainer.AcquireWriteLock()
		defer e.blockContainer.ReleaseWriteLock()

		if e.blockContainer.LastCachedBlock() < block.Number {
			// we are not out of sync, it's a sequential add of new block
			if err := e.blockContainer.AddBlock(block); err != nil {
				return err
			}
		}

		// check if some blocks reached confirmation level so that we can process their logs
		return e.processLogs()
	}

	// we are out of sync (either we missed some blocks, or a reorg happened)
	// so we get remove the old pending state and get the new one
	return e.getNewState(block)
}

// syncOnStart is a method in the EventTracker struct that is responsible
// for syncing the event tracker on startup.
// It retrieves the latest block and checks if the event tracker is out of sync.
// If it is out of sync, it calls the getNewState method to update the state.
//
// Returns:
//   - nil if sync passes successfully, or no sync is done.
//   - An error if there is an error retrieving blocks or logs from the external provider or saving logs to the store.
func (e *EventTracker) syncOnStart() (err error) {
	var latestBlock *ethgo.Block

	e.once.Do(func() {
		e.config.Logger.Info("Syncing up on start...")
		latestBlock, err = e.config.BlockProvider.GetBlockByNumber(ethgo.Latest, false)
		if err != nil {
			return
		}

		if !e.blockContainer.IsOutOfSync(latestBlock) {
			e.config.Logger.Info("Everything synced up on start")

			return
		}

		err = e.getNewState(latestBlock)
	})

	return err
}

// getNewState is called if tracker is out of sync (it missed some blocks),
// or a reorg happened in the tracked chain.
// It acquires write lock on the block container, so that the state is not changed while it
// retrieves the new blocks (new state).
// It will clean the previously cached state (non confirmed blocks), get the new state,
// set it on the block container and process logs on the confirmed blocks on the new state
//
// Input:
//   - latestBlock - latest block on the tracked chain
//
// Returns:
//   - nil if there are no confirmed blocks.
//   - An error if there is an error retrieving blocks or logs from the external provider or saving logs to the store.
func (e *EventTracker) getNewState(latestBlock *ethgo.Block) error {
	lastProcessedBlock := e.blockContainer.LastProcessedBlock()

	e.config.Logger.Info("Getting new state, since some blocks were missed",
		"lastProcessedBlock", lastProcessedBlock, "latestBlockFromRpc", latestBlock.Number)

	e.blockContainer.AcquireWriteLock()
	defer e.blockContainer.ReleaseWriteLock()

	// clean old state
	e.blockContainer.CleanState()

	startBlock := lastProcessedBlock + 1

	// sanitize startBlock from which we will start polling for blocks
	if latestBlock.Number > e.config.MaxBacklogSize &&
		latestBlock.Number-e.config.MaxBacklogSize > lastProcessedBlock {
		startBlock = latestBlock.Number - e.config.MaxBacklogSize
	}

	// get blocks in batches
	for i := startBlock; i < latestBlock.Number; i += e.config.SyncBatchSize {
		end := i + e.config.SyncBatchSize - 1
		if end > latestBlock.Number {
			// we go until the latest block, since we don't need to
			// query for it using an rpc point, since we already have it
			end = latestBlock.Number - 1
		}

		e.config.Logger.Info("Getting new state for block batch", "fromBlock", i, "toBlock", end)

		// get and add blocks in batch
		for j := i; j <= end; j++ {
			block, err := e.config.BlockProvider.GetBlockByNumber(ethgo.BlockNumber(j), false)
			if err != nil {
				e.config.Logger.Error("Getting new state for block batch failed on rpc call",
					"fromBlock", i,
					"toBlock", end,
					"currentBlock", j,
					"err", err)

				return err
			}

			if err := e.blockContainer.AddBlock(block); err != nil {
				return nil
			}
		}

		// now process logs from confirmed blocks if any
		if err := e.processLogs(); err != nil {
			return err
		}
	}

	// add latest block
	if err := e.blockContainer.AddBlock(latestBlock); err != nil {
		return err
	}

	// process logs if there are more confirmed events
	if err := e.processLogs(); err != nil {
		e.config.Logger.Error("Getting new state failed",
			"lastProcessedBlock", lastProcessedBlock,
			"latestBlockFromRpc", latestBlock.Number,
			"err", err)

		return err
	}

	e.config.Logger.Info("Getting new state finished",
		"newLastProcessedBlock", e.blockContainer.LastProcessedBlockLocked(),
		"latestBlockFromRpc", latestBlock.Number)

	return nil
}

// ProcessLogs retrieves logs for confirmed blocks, filters them based on certain criteria,
// passes them to the subscriber, and stores them in a store.
// It also removes the processed blocks from the block container.
//
// Returns:
// - nil if there are no confirmed blocks.
// - An error if there is an error retrieving logs from the external provider or saving logs to the store.
func (e *EventTracker) processLogs() error {
	confirmedBlocks := e.blockContainer.GetConfirmedBlocks(e.config.NumBlockConfirmations)
	if confirmedBlocks == nil {
		// no confirmed blocks, so nothing to process
		e.config.Logger.Debug("No confirmed blocks. Nothing to process")

		return nil
	}

	fromBlock := confirmedBlocks[0]
	toBlock := confirmedBlocks[len(confirmedBlocks)-1]

	e.config.Logger.Debug("Processing logs for blocks", "fromBlock", fromBlock, "toBlock", toBlock)

	logs, err := e.config.BlockProvider.GetLogs(e.getLogsQuery(fromBlock, toBlock))
	if err != nil {
		e.config.Logger.Error("Process logs failed on getting logs from rpc",
			"fromBlock", fromBlock,
			"toBlock", toBlock,
			"err", err)

		return err
	}

	filteredLogs := make([]*ethgo.Log, 0, len(logs))

	for _, log := range logs {
		logIDs, exist := e.config.LogFilter[log.Address]
		if !exist {
			continue
		}

		for _, id := range logIDs {
			if log.Topics[0] == id {
				filteredLogs = append(filteredLogs, log)

				if err := e.config.EventSubscriber.AddLog(log); err != nil {
					// we will only log this, since the store will have these logs
					// and subscriber can just get what he missed from store
					e.config.Logger.Error("An error occurred while passing event log to subscriber",
						"err", err)
				}

				break
			}
		}
	}

	if err := e.config.Store.InsertLastProcessedBlock(toBlock); err != nil {
		e.config.Logger.Error("Process logs failed on saving last processed block",
			"fromBlock", fromBlock,
			"toBlock", toBlock,
			"err", err)

		return err
	}

	if err := e.config.Store.InsertLogs(filteredLogs); err != nil {
		e.config.Logger.Error("Process logs failed on saving logs to store",
			"fromBlock", fromBlock,
			"toBlock", toBlock,
			"err", err)

		return err
	}

	if err := e.blockContainer.RemoveBlocks(fromBlock, toBlock); err != nil {
		return fmt.Errorf("could not remove processed blocks. Err: %w", err)
	}

	e.config.Logger.Debug("Processing logs for blocks finished",
		"fromBlock", fromBlock,
		"toBlock", toBlock,
		"numOfLogs", len(filteredLogs))

	return nil
}

// getLogsQuery is a method of the EventTracker struct that creates and returns
// a LogFilter object with the specified block range.
//
// Input:
//   - from (uint64): The starting block number for the log filter.
//   - to (uint64): The ending block number for the log filter.
//
// Returns:
//   - filter (*ethgo.LogFilter): The created LogFilter object with the specified block range.
func (e *EventTracker) getLogsQuery(from, to uint64) *ethgo.LogFilter {
	addresses := make([]ethgo.Address, 0, len(e.config.LogFilter))
	for a := range e.config.LogFilter {
		addresses = append(addresses, a)
	}

	filter := &ethgo.LogFilter{Address: addresses}
	filter.SetFromUint64(from)
	filter.SetToUint64(to)

	return filter
}
