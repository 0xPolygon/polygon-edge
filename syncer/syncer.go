package syncer

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/0xPolygon/polygon-edge/helper/progress"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
)

const (
	LoggerName  = "syncer"
	SyncerProto = "/syncer/0.2"
)

var (
	errTimeout = errors.New("timeout")
)

// XXX: Don't use this syncer for the consensus that may cause fork. This syncer doesn't assume fork. Consensus may be broken.
// TODO: Add extensibility for fork before merge
type syncer struct {
	logger          hclog.Logger
	blockchain      Blockchain
	syncProgression Progression

	peerMap         *PeerMap
	syncPeerService SyncPeerService
	syncPeerClient  SyncPeerClient

	// Timeout for syncing a block
	blockTimeout time.Duration
}

func NewSyncer(
	logger hclog.Logger,
	network Network,
	blockchain Blockchain,
	blockTimeout time.Duration,
) Syncer {
	return &syncer{
		logger:          logger.Named(LoggerName),
		blockchain:      blockchain,
		syncProgression: progress.NewProgressionWrapper(progress.ChainSyncBulk),
		syncPeerService: NewSyncPeerService(network, blockchain),
		syncPeerClient:  NewSyncPeerClient(logger, network, blockchain),
		blockTimeout:    blockTimeout,
	}
}

func (s *syncer) Start() {
	s.syncPeerClient.Start()
	s.syncPeerService.Start()

	go s.initializePeerMap()
	go s.startPeerDisconnectEventProcess()
}

func (s *syncer) initializePeerMap() {
	peerStatuses := s.syncPeerClient.GetConnectedPeerStatuses()
	s.peerMap = NewPeerMap(peerStatuses)

	for peerStatus := range s.syncPeerClient.GetPeerStatusUpdateCh() {
		s.peerMap.Put(peerStatus)
	}
}

func (s *syncer) startPeerDisconnectEventProcess() {
	for peerID := range s.syncPeerClient.GetPeerDisconnectCh() {
		s.peerMap.Delete(peerID)
	}
}

func (s *syncer) GetSyncProgression() *progress.Progression {
	return s.syncProgression.GetProgression()
}

// HasSyncPeer returns whether syncer has the peer to syncs blocks
// return false if syncer has no peer whose latest block height doesn't exceed local height
func (s *syncer) HasSyncPeer() bool {
	bestPeer := s.peerMap.BestPeer(nil)
	header := s.blockchain.Header()

	return bestPeer != nil && bestPeer.Number > header.Number
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

	skipList := make(map[string]bool)

	for {
		bestPeer := s.peerMap.BestPeer(skipList)
		if bestPeer == nil || bestPeer.Number <= localLatest {
			break
		}

		// Set the target height
		s.syncProgression.UpdateHighestProgression(bestPeer.Number)

		lastNumber, err := s.bulkSyncWithPeer(bestPeer.ID, newBlockCallback)
		if err != nil {
			s.logger.Warn("failed to complete bulk sync with peer, try to next one", "peer ID", bestPeer.ID)
		}

		if err != nil || lastNumber < bestPeer.Number {
			skipList[bestPeer.ID] = true

			// continue to next peer
			continue
		}
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

func (s *syncer) bulkSyncWithPeer(peerID string, newBlockCallback func(*types.Block)) (uint64, error) {
	localLatest := s.blockchain.Header().Number

	blockCh, err := s.syncPeerClient.GetBlocks(context.Background(), peerID, localLatest)
	if err != nil {
		return 0, err
	}

	var lastReceivedNumber uint64

	for {
		select {
		case block, ok := <-blockCh:
			if !ok {
				return lastReceivedNumber, nil
			}

			if err := s.blockchain.VerifyFinalizedBlock(block); err != nil {
				return lastReceivedNumber, fmt.Errorf("unable to verify block, %w", err)
			}

			if err := s.blockchain.WriteBlock(block); err != nil {
				return lastReceivedNumber, fmt.Errorf("failed to write block while bulk syncing: %w", err)
			}

			newBlockCallback(block)

			lastReceivedNumber = block.Number()
		case <-time.After(s.blockTimeout):
			return lastReceivedNumber, errTimeout
		}
	}
}
