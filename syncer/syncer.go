package syncer

import (
	"context"
	"fmt"

	"github.com/0xPolygon/polygon-edge/helper/progress"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
)

const (
	LoggerName  = "syncer"
	SyncerProto = "/syncer/0.2"
)

// XXX: Don't use this syncer for the consensus that may cause fork. This syncer doesn't assume fork. Consensus may be broken
// TODO: Add extensibility for fork before merge
type syncer struct {
	logger          hclog.Logger
	blockchain      Blockchain
	syncProgression Progression

	peerHeap        *PeerHeap
	syncPeerService SyncPeerService
	syncPeerClient  SyncPeerClient
}

func NewSyncer(
	logger hclog.Logger,
	blockchain Blockchain,
	network Network,
) Syncer {
	return &syncer{
		logger:          logger.Named(LoggerName),
		blockchain:      blockchain,
		syncProgression: progress.NewProgressionWrapper(progress.ChainSyncBulk),
		syncPeerService: NewSyncPeerService(network, blockchain),
		syncPeerClient:  NewSyncPeerClient(logger, network, blockchain),
	}
}

func (s *syncer) Start() {
	s.syncPeerService.Start()

	go s.startPeerHeapProcess()
}

func (s *syncer) startPeerHeapProcess() {
	peerStatuses := s.syncPeerClient.GetConnectedPeerStatuses()
	s.peerHeap = NewPeerHeap(peerStatuses)

	for peerStatus := range s.syncPeerClient.GetPeerStatusChangeCh() {
		s.peerHeap.Put(peerStatus)
	}
}

func (s *syncer) GetSyncProgression() *progress.Progression {
	return s.syncProgression.GetProgression()
}

func (s *syncer) BestPeer() *NoForkPeer {
	return s.peerHeap.BestPeer()
}

// HasSyncPeer returns whether syncer has the peer to syncs blocks
// return false if syncer has no peer whose latest block height doesn't exceed local height
func (s *syncer) HasSyncPeer() bool {
	return s.peerHeap.BestPeer() != nil
}

func (s *syncer) BulkSync(ctx context.Context, newBlockCallback func(*types.Block)) error {
	localLatest := uint64(0)
	if header := s.blockchain.Header(); header != nil {
		localLatest = header.Number
	}

	// Create a blockchain subscription for the sync progression and start tracking
	s.syncProgression.StartProgression(localLatest, s.blockchain.SubscribeEvents())

	// Stop monitoring the sync progression upon exit
	defer s.syncProgression.StopProgression()

	for {
		bestPeer := s.peerHeap.BestPeer()
		if bestPeer == nil || bestPeer.Number <= localLatest {
			break
		}

		blockCh, err := s.syncPeerClient.GetBlocks(ctx, bestPeer.ID, localLatest)
		if err != nil {
			return err
		}

		// Set the target height
		s.syncProgression.UpdateHighestProgression(bestPeer.Number)

		var lastReceivedNumber uint64

		for block := range blockCh {
			if err := s.blockchain.VerifyFinalizedBlock(block); err != nil {
				return fmt.Errorf("unable to verify block, %w", err)
			}

			if err := s.blockchain.WriteBlock(block); err != nil {
				return fmt.Errorf("failed to write block while bulk syncing: %w", err)
			}

			newBlockCallback(block)

			lastReceivedNumber = block.Number()
		}

		// when can fetch blocks to the latest, then return
		if lastReceivedNumber >= bestPeer.Number {
			break
		}

		// remove because peer failed to sync
		// TODO: check this
		s.peerHeap.Remove(bestPeer.ID)
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
