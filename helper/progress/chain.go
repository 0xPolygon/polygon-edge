package progress

import (
	"sync"

	"github.com/0xPolygon/polygon-edge/blockchain"
)

type ChainSyncType string

const (
	ChainSyncRestore ChainSyncType = "restore"
	ChainSyncBulk    ChainSyncType = "bulk-sync"
)

// Progression defines the status of the sync
// progression of the node
type Progression struct {
	// SyncType is indicating the sync method
	SyncType ChainSyncType

	// StartingBlock is the initial block that the node is starting
	// the sync from. It is reset after every sync batch
	StartingBlock uint64

	// CurrentBlock is the last written block from the sync batch
	CurrentBlock uint64

	// HighestBlock is the target block in the sync batch
	HighestBlock uint64
}

type ProgressionWrapper struct {
	// progression is a reference to the ongoing batch sync.
	// Nil if no batch sync is currently in progress
	progression *Progression

	// stopCh is the channel for receiving stop signals
	// in progression tracking
	stopCh chan struct{}

	lock sync.RWMutex

	syncType ChainSyncType
}

func NewProgressionWrapper(syncType ChainSyncType) *ProgressionWrapper {
	return &ProgressionWrapper{
		progression: nil,
		stopCh:      make(chan struct{}),
		syncType:    syncType,
	}
}

// startProgression initializes the progression tracking
func (pw *ProgressionWrapper) StartProgression(
	startingBlock uint64,
	subscription blockchain.Subscription,
) {
	pw.lock.Lock()
	defer pw.lock.Unlock()

	pw.progression = &Progression{
		SyncType:      pw.syncType,
		StartingBlock: startingBlock,
	}

	go pw.RunUpdateLoop(subscription)
}

// runUpdateLoop starts the blockchain event monitoring loop and
// updates the currently written block in the batch sync
func (pw *ProgressionWrapper) RunUpdateLoop(subscription blockchain.Subscription) {
	eventCh := subscription.GetEventCh()

	for {
		select {
		case event := <-eventCh:
			if event.Type == blockchain.EventFork {
				continue
			}

			if len(event.NewChain) == 0 {
				continue
			}

			lastBlock := event.NewChain[len(event.NewChain)-1]
			pw.UpdateCurrentProgression(lastBlock.Number)
		case <-pw.stopCh:
			subscription.Close()

			return
		}
	}
}

// StopProgression stops the progression tracking
func (pw *ProgressionWrapper) StopProgression() {
	pw.stopCh <- struct{}{}

	pw.lock.Lock()
	defer pw.lock.Unlock()

	pw.progression = nil
}

// UpdateCurrentProgression sets the currently written block in the bulk sync
func (pw *ProgressionWrapper) UpdateCurrentProgression(currentBlock uint64) {
	pw.lock.Lock()
	defer pw.lock.Unlock()

	pw.progression.CurrentBlock = currentBlock
}

// UpdateHighestProgression sets the highest-known target block in the bulk sync
func (pw *ProgressionWrapper) UpdateHighestProgression(highestBlock uint64) {
	pw.lock.Lock()
	defer pw.lock.Unlock()

	pw.progression.HighestBlock = highestBlock
}

// GetProgression returns the latest sync progression
func (pw *ProgressionWrapper) GetProgression() *Progression {
	pw.lock.RLock()
	defer pw.lock.RUnlock()

	return pw.progression
}
