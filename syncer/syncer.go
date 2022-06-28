package syncer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/0xPolygon/polygon-edge/helper/progress"
	"github.com/0xPolygon/polygon-edge/network/event"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p-core/peer"
)

const (
	LoggerName  = "syncer"
	SyncerProto = "/syncer/0.2"
)

var (
	errTimeout = errors.New("timeout")
)

// XXX: Don't use this syncer for the consensus that may cause fork.
// This syncer doesn't assume fork. Consensus may be broken.
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

	// Channel to notify WatchSync that a new status arrived
	newStatusCh chan struct{}

	// XXX: Experiment Code Begins
	syncTo *uint64
	// XXX: Experiment Code Ends
}

func NewSyncer(
	logger hclog.Logger,
	network Network,
	blockchain Blockchain,
	blockTimeout time.Duration,
) Syncer {
	// XXX: Experiment Code Begins
	var syncTo *uint64

	if e := os.Getenv("SYNC_TO"); e != "" {
		n, err := strconv.ParseInt(e, 10, 64)
		if err != nil {
			logger.Error("failed to parse uint64 string", "SYNC_TO", e)

			return nil
		}

		if n < 0 {
			logger.Error("SYNC_TO must be positive", "n", n)

			return nil
		}

		x := uint64(n)

		syncTo = &x

		fmt.Printf("SyncTo => %d\n", x)
	} else {
		fmt.Printf("SyncTo => latest \n")
	}
	// XXX: Experiment Code Ends

	return &syncer{
		logger:          logger.Named(LoggerName),
		blockchain:      blockchain,
		syncProgression: progress.NewProgressionWrapper(progress.ChainSyncBulk),
		syncPeerService: NewSyncPeerService(network, blockchain),
		syncPeerClient:  NewSyncPeerClient(logger, network, blockchain),
		blockTimeout:    blockTimeout,
		newStatusCh:     make(chan struct{}),
		peerMap:         new(PeerMap),

		// XXX: Experiment Code Begins
		syncTo: syncTo,
		// XXX: Experiment Code Ends
	}
}

func (s *syncer) Start() error {
	if err := s.syncPeerClient.Start(); err != nil {
		return err
	}

	s.syncPeerService.Start()

	go s.initializePeerMap()
	go s.startPeerConnectionEventProcess()

	return nil
}

func (s *syncer) initializePeerMap() {
	peerStatuses := s.syncPeerClient.GetConnectedPeerStatuses()
	s.peerMap.PutPeers(peerStatuses)

	for peerStatus := range s.syncPeerClient.GetPeerStatusUpdateCh() {
		s.peerMap.Put(peerStatus)

		select {
		case s.newStatusCh <- struct{}{}:
		default:
		}
	}
}

func (s *syncer) startPeerConnectionEventProcess() {
	for e := range s.syncPeerClient.GetPeerConnectionUpdateEventCh() {
		peerID := e.PeerID

		switch e.Type {
		case event.PeerConnected:
			go func() {
				status, err := s.syncPeerClient.GetPeerStatus(peerID)
				if err != nil {
					s.logger.Warn("failed to get peer status, skip", "id", peerID, "err", err)

					return
				}

				s.peerMap.Put(status)

				select {
				case s.newStatusCh <- struct{}{}:
				default:
				}
			}()
		case event.PeerDisconnected:
			s.peerMap.Remove(peerID)
		}
	}
}

func (s *syncer) GetSyncProgression() *progress.Progression {
	return s.syncProgression.GetProgression()
}

// HasSyncPeer returns whether syncer has the peer to syncs blocks
// return false if syncer has no peer whose latest block height doesn't exceed local height
func (s *syncer) HasSyncPeer() bool {
	if s.peerMap == nil {
		return false
	}

	bestPeer := s.peerMap.BestPeer(nil)
	header := s.blockchain.Header()

	return bestPeer != nil && bestPeer.Number > header.Number
}

func (s *syncer) BulkSync(ctx context.Context, newBlockCallback func(*types.Block) bool) error {
	localLatest := uint64(0)
	updateLocalLatest := func() {
		if header := s.blockchain.Header(); header != nil {
			localLatest = header.Number
		}
	}

	updateLocalLatest()

	// Create a blockchain subscription for the sync progression and start tracking
	s.syncProgression.StartProgression(localLatest+1, s.blockchain.SubscribeEvents())

	// Stop monitoring the sync progression upon exit
	defer s.syncProgression.StopProgression()

	skipList := make(map[peer.ID]bool)

	for {
		bestPeer := s.peerMap.BestPeer(skipList)
		if bestPeer == nil || bestPeer.Number <= localLatest {
			break
		}

		// Set the target height
		s.syncProgression.UpdateHighestProgression(bestPeer.Number)

		lastNumber, _, err := s.bulkSyncWithPeer(bestPeer.ID, newBlockCallback)
		if err != nil {
			s.logger.Warn("failed to complete bulk sync with peer, try to next one", "peer ID", bestPeer.ID)
		}

		// if node could sync with the peer fully, then exit loop
		if err == nil && lastNumber >= bestPeer.Number {
			break
		}

		updateLocalLatest()

		skipList[bestPeer.ID] = true
	}

	return nil
}

func (s *syncer) WatchSync(ctx context.Context, callback func(*types.Block) bool) error {
	localLatest := s.blockchain.Header().Number
	skipList := make(map[peer.ID]bool)

	// Loop until context is canceled
	for {
		//Wait for a new event to arrive
		<-s.newStatusCh

		// fetch local latest block
		if header := s.blockchain.Header(); header != nil {
			localLatest = header.Number
		}

		// pick one best peer
		bestPeer := s.peerMap.BestPeer(skipList)
		if bestPeer == nil {
			// Empty skipList map if there are no best peers
			skipList = make(map[peer.ID]bool)

			continue
		}

		// if the bestPeer does not have a new block continue
		if bestPeer.Number <= localLatest {
			continue
		}

		// fetch block from the peer
		lastNumber, shouldTerminate, err := s.bulkSyncWithPeer(bestPeer.ID, callback)
		if err != nil {
			s.logger.Warn("failed to complete bulk sync with peer, try to next one", "peer ID", bestPeer.ID)
		}

		if err != nil || lastNumber < bestPeer.Number {
			skipList[bestPeer.ID] = true

			// continue to next peer
			continue
		}

		if shouldTerminate {
			break
		}
	}

	return nil
}

func (s *syncer) bulkSyncWithPeer(peerID peer.ID, newBlockCallback func(*types.Block) bool) (uint64, bool, error) {
	localLatest := s.blockchain.Header().Number

	// XXX: Experiment Code Begins
	success := false
	fmt.Printf("\n\n### Start bulkSyncWithPeer with %s ### \n\n", peerID.String())
	start := time.Now()

	defer func() {
		currentLatest := s.blockchain.Header().Number
		end := time.Now()

		fmt.Printf("\n\n### Experiment Result ###\n")
		fmt.Printf("From: %d\n", localLatest+1)
		fmt.Printf("To: %d\n", currentLatest)
		fmt.Printf("Number: %d\n", currentLatest-localLatest)
		fmt.Printf("Time: %f [sec]\n", end.Sub(start).Seconds())
		fmt.Println("")

		if success {
			os.Exit(0)
		}
	}()
	// XXX: Experiment Code Ends

	shouldTerminate := false

	blockCh, err := s.syncPeerClient.GetBlocks(context.Background(), peerID, localLatest+1)
	if err != nil {
		return 0, false, err
	}

	defer func() {
		err := s.syncPeerClient.CloseStream(peerID)
		if err != nil {
			s.logger.Error("Failed to close stream: ", err)
		}
	}()

	var lastReceivedNumber uint64

	for {
		select {
		case block, ok := <-blockCh:
			if !ok {
				success = true

				return lastReceivedNumber, shouldTerminate, nil
			}

			// safe check
			if block.Number() == 0 {
				continue
			}

			if err := s.blockchain.VerifyFinalizedBlock(block); err != nil {
				return lastReceivedNumber, false, fmt.Errorf("unable to verify block, %w", err)
			}

			if err := s.blockchain.WriteBlock(block); err != nil {
				return lastReceivedNumber, false, fmt.Errorf("failed to write block while bulk syncing: %w", err)
			}

			shouldTerminate = newBlockCallback(block)

			lastReceivedNumber = block.Number()

			// XXX: Experiment Code Begins
			if s.syncTo != nil && lastReceivedNumber == *s.syncTo {
				success = true

				return lastReceivedNumber, shouldTerminate, errTimeout
			}
			// XXX: Experiment Code Ends

		case <-time.After(s.blockTimeout):
			return lastReceivedNumber, shouldTerminate, errTimeout
		}
	}
}
