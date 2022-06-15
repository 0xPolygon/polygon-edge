package syncer

import (
	"context"
	"fmt"

	"github.com/0xPolygon/polygon-edge/blockchain"
	"github.com/0xPolygon/polygon-edge/helper/progress"
	"github.com/0xPolygon/polygon-edge/syncer/peers"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
)

const (
	LoggerName = "syncer"
)

type Syncer interface {
	Start()
	GetSyncProgression() *progress.Progression
	BestPeer() *peers.BestPeer
	HasSyncPeer() bool
	BulkSync(context.Context, func(*types.Block)) error
	WatchSync(context.Context, func(*types.Block) bool) error
}

type Blockchain interface {
	// Subscribe new block event
	SubscribeEvents() blockchain.Subscription
	// Get latest header
	Header() *types.Header
	// Verify fetched block
	VerifyFinalizedBlock(*types.Block) error
	// Write block to chain
	WriteBlock(*types.Block) error
}

type Progression interface {
	StartProgression(startingBlock uint64, subscription blockchain.Subscription)
	UpdateHighestProgression(highestBlock uint64)
	GetProgression() *progress.Progression
	StopProgression()
}

type syncer struct {
	logger          hclog.Logger
	blockchain      Blockchain
	syncProgression Progression
	syncPeers       peers.SyncPeers
}

func NewSyncer(
	logger hclog.Logger,
	blockchain Blockchain,
	syncPeers peers.SyncPeers,
) Syncer {
	return &syncer{
		logger:          logger.Named(LoggerName),
		blockchain:      blockchain,
		syncProgression: progress.NewProgressionWrapper(progress.ChainSyncBulk),
		syncPeers:       syncPeers,
	}
}

func (s *syncer) Start() {
	s.syncPeers.Start()
}

func (s *syncer) GetSyncProgression() *progress.Progression {
	return s.syncProgression.GetProgression()
}

func (s *syncer) BestPeer() *peers.BestPeer {
	return s.syncPeers.BestPeer()
}

// HasSyncPeer returns whether syncer has the peer to syncs blocks
// return false if syncer has no peer whose latest block height doesn't exceed local height
func (s *syncer) HasSyncPeer() bool {
	return s.syncPeers.BestPeer() != nil
}

func (s *syncer) BulkSync(ctx context.Context, newBlockCallback func(*types.Block)) error {
	localLatest := uint64(0)
	if header := s.blockchain.Header(); header != nil {
		localLatest = header.Number
	}

	bestPeer := s.syncPeers.BestPeer()
	if bestPeer == nil || bestPeer.Number <= localLatest {
		return nil
	}

	blockCh, err := s.syncPeers.GetBlocks(ctx, bestPeer.ID, localLatest)
	if err != nil {
		return err
	}

	// Create a blockchain subscription for the sync progression and start tracking
	s.syncProgression.StartProgression(localLatest, s.blockchain.SubscribeEvents())

	// Stop monitoring the sync progression upon exit
	defer s.syncProgression.StopProgression()

	// Set the target height
	s.syncProgression.UpdateHighestProgression(bestPeer.Number)

	for block := range blockCh {
		if err := s.blockchain.VerifyFinalizedBlock(block); err != nil {
			return fmt.Errorf("unable to verify block, %w", err)
		}

		if err := s.blockchain.WriteBlock(block); err != nil {
			return fmt.Errorf("failed to write block while bulk syncing: %w", err)
		}

		newBlockCallback(block)
	}

	return nil
}

func (s *syncer) WatchSync(ctx context.Context, callback func(*types.Block) bool) error {
	// Loop until context is canceled
	for {
		// pick one best peer
		// peer := s.peerHeap.BestPeer()

		// fetch block from the peer

		// verify the block

		// write the block

		select {
		case <-ctx.Done():
			// context is canceled
			break
		}
	}
}
