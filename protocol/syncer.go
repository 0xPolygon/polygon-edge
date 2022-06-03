package protocol

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p-core/peer"
	anypb "google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/0xPolygon/polygon-edge/blockchain"
	"github.com/0xPolygon/polygon-edge/helper/progress"
	"github.com/0xPolygon/polygon-edge/network"
	"github.com/0xPolygon/polygon-edge/network/event"
	libp2pGrpc "github.com/0xPolygon/polygon-edge/network/grpc"
	"github.com/0xPolygon/polygon-edge/protocol/proto"
	"github.com/0xPolygon/polygon-edge/types"
)

const (
	maxEnqueueSize = 50
	popTimeout     = 10 * time.Second
)

var (
	ErrPopTimeout       = errors.New("timeout")
	ErrConnectionClosed = errors.New("connection closed")
)

// Syncer is a sync protocol
type Syncer struct {
	logger     hclog.Logger
	blockchain blockchainShim

	peers sync.Map // Maps peer.ID -> SyncPeer

	serviceV1 *serviceV1
	stopCh    chan struct{}

	status     *Status
	statusLock sync.Mutex

	server *network.Server

	syncProgression *progress.ProgressionWrapper
}

// NewSyncer creates a new Syncer instance
func NewSyncer(logger hclog.Logger, server *network.Server, blockchain blockchainShim) *Syncer {
	s := &Syncer{
		logger:          logger.Named("syncer"),
		stopCh:          make(chan struct{}),
		blockchain:      blockchain,
		server:          server,
		syncProgression: progress.NewProgressionWrapper(progress.ChainSyncBulk),
	}

	return s
}

// GetSyncProgression returns the latest sync progression, if any
func (s *Syncer) GetSyncProgression() *progress.Progression {
	return s.syncProgression.GetProgression()
}

// syncCurrentStatus taps into the blockchain event steam and updates the Syncer.status field
func (s *Syncer) syncCurrentStatus() {
	sub := s.blockchain.SubscribeEvents()
	eventCh := sub.GetEventCh()

	// watch the subscription and notify
	for {
		select {
		case evnt := <-eventCh:
			if evnt.Type == blockchain.EventFork {
				// we do not want to notify forks
				continue
			}

			if len(evnt.NewChain) == 0 {
				// this should not happen
				continue
			}

			status := &Status{
				Difficulty: evnt.Difficulty,
				Hash:       evnt.NewChain[0].Hash,
				Number:     evnt.NewChain[0].Number,
			}

			s.statusLock.Lock()
			s.status = status
			s.statusLock.Unlock()

		case <-s.stopCh:
			sub.Close()

			return
		}
	}
}

const syncerV1 = "/syncer/0.1"

// enqueueBlock adds the specific block to the peerID queue
func (s *Syncer) enqueueBlock(peerID peer.ID, b *types.Block) {
	s.logger.Debug("enqueue block", "peer", peerID, "number", b.Number(), "hash", b.Hash())

	peer, exists := s.peers.Load(peerID)
	if !exists {
		s.logger.Error("enqueue block: peer not present", "id", peerID.String())

		return
	}

	syncPeer, ok := peer.(*SyncPeer)
	if !ok {
		s.logger.Error("invalid sync peer type cast")

		return
	}

	syncPeer.appendBlock(b)
}

func (s *Syncer) updatePeerStatus(peerID peer.ID, status *Status) {
	s.logger.Debug(
		"update peer status",
		"peer",
		peerID,
		"latest block number",
		status.Number,
		"latest block hash",
		status.Hash,
		"difficulty",
		status.Difficulty,
	)

	if peer, ok := s.peers.Load(peerID); ok {
		syncPeer, ok := peer.(*SyncPeer)
		if !ok {
			s.logger.Error("invalid sync peer type cast")

			return
		}

		syncPeer.updateStatus(status)
	}
}

// Broadcast broadcasts a block to all peers
func (s *Syncer) Broadcast(b *types.Block) {
	// Get the chain difficulty associated with block
	td, ok := s.blockchain.GetTD(b.Hash())
	if !ok {
		// not supposed to happen
		s.logger.Error("total difficulty not found", "block number", b.Number())

		return
	}

	// broadcast the new block to all the peers
	req := &proto.NotifyReq{
		Status: &proto.V1Status{
			Hash:       b.Hash().String(),
			Number:     b.Number(),
			Difficulty: td.String(),
		},
		Raw: &anypb.Any{
			Value: b.MarshalRLP(),
		},
	}

	//	notify peers in the background
	go s.notifyPeers(req)
}

func (s *Syncer) notifyPeers(req *proto.NotifyReq) {
	s.peers.Range(func(key, value interface{}) bool {
		peerID, _ := key.(peer.ID)
		syncPeer, _ := value.(*SyncPeer)

		startTime := time.Now()
		if _, err := syncPeer.client.Notify(context.Background(), req); err != nil {
			s.logger.Error("failed to notify", "err", err)
		}

		duration := time.Since(startTime)

		s.logger.Debug(
			"notifying peer",
			"id", peerID.String(),
			"duration", duration.Seconds(),
		)

		return true
	})
}

// Start starts the syncer protocol
func (s *Syncer) Start() {
	s.serviceV1 = &serviceV1{syncer: s, logger: hclog.NewNullLogger(), store: s.blockchain}

	// Get the current status of the syncer
	currentHeader := s.blockchain.Header()
	diff, _ := s.blockchain.GetTD(currentHeader.Hash)

	s.status = &Status{
		Hash:       currentHeader.Hash,
		Number:     currentHeader.Number,
		Difficulty: diff,
	}

	// Run the blockchain event listener loop
	go s.syncCurrentStatus()

	// Register the grpc protocol for syncer
	grpcStream := libp2pGrpc.NewGrpcStream()
	proto.RegisterV1Server(grpcStream.GrpcServer(), s.serviceV1)
	grpcStream.Serve()
	s.server.RegisterProtocol(syncerV1, grpcStream)

	s.setupPeers()

	go s.handlePeerEvent()
}

// setupPeers adds connected peers as syncer peers
func (s *Syncer) setupPeers() {
	for _, p := range s.server.Peers() {
		if addErr := s.AddPeer(p.Info.ID); addErr != nil {
			s.logger.Error(fmt.Sprintf("Error when adding peer [%s], %v", p.Info.ID, addErr))
		}
	}
}

// handlePeerEvent subscribes network event and adds/deletes peer from syncer
func (s *Syncer) handlePeerEvent() {
	updateCh, err := s.server.SubscribeCh()
	if err != nil {
		s.logger.Error("failed to subscribe", "err", err)

		return
	}

	go func() {
		for {
			evnt, ok := <-updateCh
			if !ok {
				return
			}

			switch evnt.Type {
			case event.PeerConnected:
				if err := s.AddPeer(evnt.PeerID); err != nil {
					s.logger.Error("failed to add peer", "err", err)
				}
			case event.PeerDisconnected:
				if err := s.DeletePeer(evnt.PeerID); err != nil {
					s.logger.Error("failed to delete user", "err", err)
				}
			}
		}
	}()
}

// BestPeer returns the best peer by block height (if any)
func (s *Syncer) BestPeer() *SyncPeer {
	var (
		bestPeer        *SyncPeer
		bestBlockNumber uint64
	)

	// Find the peer with the biggest block height available
	s.peers.Range(func(peerID, peer interface{}) bool {
		syncPeer, ok := peer.(*SyncPeer)
		if !ok {
			return false
		}

		peerBlockNumber := syncPeer.Number()
		if bestPeer == nil || peerBlockNumber > bestBlockNumber {
			// There is currently no best peer set, or the peer's block number
			// is currently the highest
			bestPeer = syncPeer
			bestBlockNumber = peerBlockNumber
		}

		return true
	})

	// Fetch the highest local block height
	if bestBlockNumber <= s.blockchain.Header().Number {
		bestPeer = nil
	}

	return bestPeer
}

// AddPeer establishes new connection with the given peer
func (s *Syncer) AddPeer(peerID peer.ID) error {
	if _, ok := s.peers.Load(peerID); ok {
		// already connected
		return nil
	}

	stream, err := s.server.NewStream(syncerV1, peerID)
	if err != nil {
		return fmt.Errorf("failed to open a stream, err %w", err)
	}

	conn := libp2pGrpc.WrapClient(stream)

	// watch for changes of the other node first
	clt := proto.NewV1Client(conn)

	rawStatus, err := clt.GetCurrent(context.Background(), &emptypb.Empty{})
	if err != nil {
		return err
	}

	status, err := statusFromProto(rawStatus)

	if err != nil {
		return err
	}

	s.peers.Store(peerID, &SyncPeer{
		peer:      peerID,
		conn:      conn,
		client:    clt,
		status:    status,
		enqueueCh: make(chan struct{}),
	})

	return nil
}

// DeletePeer deletes a peer from syncer
func (s *Syncer) DeletePeer(peerID peer.ID) error {
	p, ok := s.peers.LoadAndDelete(peerID)
	if ok {
		syncPeer, ok := p.(*SyncPeer)
		if !ok {
			return errors.New("invalid type assertion")
		}

		if err := syncPeer.conn.Close(); err != nil {
			return err
		}

		close(syncPeer.enqueueCh)
	}

	return nil
}

// WatchSyncWithPeer subscribes and adds peer's latest block
func (s *Syncer) WatchSyncWithPeer(p *SyncPeer, newBlockHandler func(b *types.Block) bool, blockTimeout time.Duration) {
	// purge from the cache of broadcasted blocks all the ones we have written so far
	header := s.blockchain.Header()
	p.purgeBlocks(header.Hash)

	// listen and enqueue the messages
	for {
		if p.IsClosed() {
			s.logger.Info("Connection to a peer has closed already", "id", p.peer)

			break
		}

		b, err := p.popBlock(blockTimeout * 3)
		if err != nil {
			s.logSyncPeerPopBlockError(err, p)

			break
		}

		if err := s.blockchain.VerifyFinalizedBlock(b); err != nil {
			s.logger.Error("unable to verify block, %w", err)

			return
		}

		if err := s.blockchain.WriteBlock(b); err != nil {
			s.logger.Error("failed to write block", "err", err)

			break
		}

		shouldExit := newBlockHandler(b)

		s.prunePeerEnqueuedBlocks(b)

		if shouldExit {
			break
		}
	}
}

func (s *Syncer) logSyncPeerPopBlockError(err error, peer *SyncPeer) {
	if errors.Is(err, ErrPopTimeout) {
		msg := "failed to pop block within %ds from peer: id=%s, please check if all the validators are running"
		s.logger.Warn(fmt.Sprintf(msg, int(popTimeout.Seconds()), peer.peer))
	} else {
		s.logger.Info("failed to pop block from peer", "id", peer.peer, "err", err)
	}
}

// BulkSyncWithPeer finds common ancestor with a peer and syncs block until latest block.
// Only missing blocks are synced up to the peer's highest block number
func (s *Syncer) BulkSyncWithPeer(p *SyncPeer, newBlockHandler func(block *types.Block)) error {
	// Find the local height
	localMaxHeader := s.blockchain.Header()
	localMaxHeight := localMaxHeader.Number

	if localMaxHeight >= p.Number() {
		// No need to sync with this peer
		// since the local chain on the node is longer
		return nil
	}

	// Create a blockchain subscription for the sync progression and start tracking
	s.syncProgression.StartProgression(localMaxHeader.Number, s.blockchain.SubscribeEvents())

	// Stop monitoring the sync progression upon exit
	defer s.syncProgression.StopProgression()

	// Keep track of the progress
	var (
		lastTarget        uint64
		currentSyncHeight = localMaxHeight + 1
	)

	// Sync up to the peer's latest header
	for {
		// Update the target. This entire outer loop
		// is there in order to make sure bulk syncing is entirely done
		// as the peer's status can change over time if block writes have a significant
		// time impact on the node in question
		target := p.Number()

		s.syncProgression.UpdateHighestProgression(target)

		if target == lastTarget {
			// there are no more changes to pull for now
			break
		}

		for {
			s.logger.Debug(
				"sync up to block",
				"from",
				currentSyncHeight,
				"to",
				target,
			)

			// Create the base request skeleton
			sk := &skeleton{
				amount: MaxSkeletonHeadersAmount,
			}

			// Fetch the blocks from the peer
			if err := sk.getBlocksFromPeer(p.client, currentSyncHeight); err != nil {
				return fmt.Errorf("unable to fetch blocks from peer, %w", err)
			}

			// Verify and write the data locally
			for _, block := range sk.blocks {
				if err := s.blockchain.VerifyFinalizedBlock(block); err != nil {
					return fmt.Errorf("unable to verify block, %w", err)
				}

				if err := s.blockchain.WriteBlock(block); err != nil {
					return fmt.Errorf("failed to write block while bulk syncing: %w", err)
				}

				newBlockHandler(block)
				s.prunePeerEnqueuedBlocks(block)
				currentSyncHeight++
			}

			if currentSyncHeight >= target {
				// Target has been reached
				break
			}
		}

		lastTarget = target
	}

	return nil
}

func (s *Syncer) prunePeerEnqueuedBlocks(block *types.Block) {
	s.peers.Range(func(key, value interface{}) bool {
		peerID, _ := key.(peer.ID)
		syncPeer, _ := value.(*SyncPeer)

		pruned := syncPeer.purgeBlocks(block.Hash())

		s.logger.Debug(
			"pruned peer enqueued block",
			"num", pruned,
			"id", peerID.String(),
			"reference_block_num", block.Number(),
		)

		return true
	})
}
