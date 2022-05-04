package protocol

import (
	"context"
	"errors"
	"fmt"
	"github.com/0xPolygon/polygon-edge/network/event"
	"math"
	"math/big"
	"sync"
	"time"

	"github.com/0xPolygon/polygon-edge/blockchain"
	"github.com/0xPolygon/polygon-edge/helper/progress"
	"github.com/0xPolygon/polygon-edge/network"
	libp2pGrpc "github.com/0xPolygon/polygon-edge/network/grpc"
	"github.com/0xPolygon/polygon-edge/protocol/proto"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p-core/peer"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	anypb "google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/emptypb"
)

const (
	maxEnqueueSize = 50
	popTimeout     = 10 * time.Second
)

var (
	ErrLoadLocalGenesisFailed = errors.New("failed to read local genesis")
	ErrMismatchGenesis        = errors.New("genesis does not match")
	ErrCommonAncestorNotFound = errors.New("header is nil")
	ErrForkNotFound           = errors.New("fork not found")
	ErrPopTimeout             = errors.New("timeout")
	ErrConnectionClosed       = errors.New("connection closed")
)

// SyncPeer is a representation of the peer the node is syncing with
type SyncPeer struct {
	peer   peer.ID
	conn   *grpc.ClientConn
	client proto.V1Client

	status     *Status
	statusLock sync.RWMutex

	enqueueLock sync.Mutex
	enqueue     []*types.Block
	enqueueCh   chan struct{}
}

// Number returns the latest peer block height
func (s *SyncPeer) Number() uint64 {
	s.statusLock.RLock()
	defer s.statusLock.RUnlock()

	return s.status.Number
}

// IsClosed returns whether peer's connectivity has been closed
func (s *SyncPeer) IsClosed() bool {
	return s.conn.GetState() == connectivity.Shutdown
}

// purgeBlocks purges the cache of broadcasted blocks the node has written so far
// from the SyncPeer
func (s *SyncPeer) purgeBlocks(lastSeen types.Hash) {
	s.enqueueLock.Lock()
	defer s.enqueueLock.Unlock()

	indx := -1

	for i, b := range s.enqueue {
		if b.Hash() == lastSeen {
			indx = i
		}
	}

	if indx != -1 {
		s.enqueue = s.enqueue[indx+1:]
	}
}

// popBlock pops a block from the block queue [BLOCKING]
func (s *SyncPeer) popBlock(timeout time.Duration) (b *types.Block, err error) {
	timeoutCh := time.After(timeout)

	for {
		if !s.IsClosed() {
			s.enqueueLock.Lock()
			if len(s.enqueue) != 0 {
				b, s.enqueue = s.enqueue[0], s.enqueue[1:]
				s.enqueueLock.Unlock()

				return
			}

			s.enqueueLock.Unlock()
			select {
			case <-s.enqueueCh:
			case <-timeoutCh:
				return nil, ErrPopTimeout
			}
		} else {
			return nil, ErrConnectionClosed
		}
	}
}

// appendBlock adds a new block to the block queue
func (s *SyncPeer) appendBlock(b *types.Block) {
	s.enqueueLock.Lock()
	defer s.enqueueLock.Unlock()

	if len(s.enqueue) == maxEnqueueSize {
		// pop first element
		s.enqueue = s.enqueue[1:]
	}
	// append to the end
	s.enqueue = append(s.enqueue, b)

	select {
	case s.enqueueCh <- struct{}{}:
	default:
	}
}

func (s *SyncPeer) updateStatus(status *Status) {
	s.statusLock.Lock()
	defer s.statusLock.Unlock()

	s.status = status
}

// Status defines the up to date information regarding the peer
type Status struct {
	Difficulty *big.Int   // Current difficulty
	Hash       types.Hash // Latest block hash
	Number     uint64     // Latest block number
}

// Copy creates a copy of the status
func (s *Status) Copy() *Status {
	ss := new(Status)
	ss.Hash = s.Hash
	ss.Number = s.Number
	ss.Difficulty = new(big.Int).Set(s.Difficulty)

	return ss
}

// toProto converts a Status object to a proto.V1Status
func (s *Status) toProto() *proto.V1Status {
	return &proto.V1Status{
		Number:     s.Number,
		Hash:       s.Hash.String(),
		Difficulty: s.Difficulty.String(),
	}
}

// fromProto converts a proto.V1Status to a Status object
func fromProto(status *proto.V1Status) (*Status, error) {
	diff, ok := new(big.Int).SetString(status.Difficulty, 10)
	if !ok {
		return nil, fmt.Errorf("failed to parse difficulty: %s", status.Difficulty)
	}

	return &Status{
		Number:     status.Number,
		Hash:       types.StringToHash(status.Hash),
		Difficulty: diff,
	}, nil
}

// statusFromProto extracts a Status object from a passed in proto.V1Status
func statusFromProto(p *proto.V1Status) (*Status, error) {
	s := new(Status)
	if err := s.Hash.UnmarshalText([]byte(p.Hash)); err != nil {
		return nil, err
	}

	s.Number = p.Number

	diff, ok := new(big.Int).SetString(p.Difficulty, 10)
	if !ok {
		return nil, fmt.Errorf("failed to decode difficulty")
	}

	s.Difficulty = diff

	return s, nil
}

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

	peer, ok := s.peers.Load(peerID)
	if ok {
		syncPeer, ok := peer.(*SyncPeer)
		if !ok {
			s.logger.Error("invalid sync peer type cast")

			return
		}

		syncPeer.appendBlock(b)
	}
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

	s.peers.Range(func(peerID, peer interface{}) bool {
		if _, err := peer.(*SyncPeer).client.Notify(context.Background(), req); err != nil {
			s.logger.Error("failed to notify", "err", err)
		}

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

		status := syncPeer.status
		if bestPeer == nil || status.Number > bestBlockNumber {
			// There is currently no best peer set, or the peer's block number
			// is currently the highest

			var correctAssertion bool

			bestPeer, correctAssertion = peer.(*SyncPeer)
			if !correctAssertion {
				return false
			}

			bestBlockNumber = status.Number
		}

		return true
	})

	// Fetch the highest local block height
	if bestBlockNumber < s.blockchain.Header().Number {
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

// findCommonAncestor returns the common ancestor header and fork
func (s *Syncer) findCommonAncestor(clt proto.V1Client, status *Status) (*types.Header, *types.Header, error) {
	h := s.blockchain.Header()

	min := uint64(0) // genesis
	max := h.Number

	targetHeight := status.Number

	if heightNumber := targetHeight; max > heightNumber {
		max = heightNumber
	}

	var header *types.Header

	for min <= max {
		m := uint64(math.Floor(float64(min+max) / 2))

		if m == 0 {
			// our common ancestor is the genesis
			genesis, ok := s.blockchain.GetHeaderByNumber(0)
			if !ok {
				return nil, nil, ErrLoadLocalGenesisFailed
			}

			header = genesis

			break
		}

		found, err := getHeader(clt, &m, nil)
		if err != nil {
			return nil, nil, err
		}

		if found == nil {
			// peer does not have the m peer, search in lower bounds
			max = m - 1
		} else {
			expectedHeader, ok := s.blockchain.GetHeaderByNumber(m)
			if !ok {
				return nil, nil, fmt.Errorf("cannot find the header %d in local chain", m)
			}
			if expectedHeader.Hash == found.Hash {
				header = found
				min = m + 1
			} else {
				if m == 0 {
					return nil, nil, ErrMismatchGenesis
				}
				max = m - 1
			}
		}
	}

	if header == nil {
		return nil, nil, ErrCommonAncestorNotFound
	}

	// get the block fork
	forkNum := header.Number + 1
	fork, err := getHeader(clt, &forkNum, nil)

	if err != nil {
		return nil, nil, fmt.Errorf("failed to get fork at num %d", header.Number)
	}

	if fork == nil {
		return nil, nil, ErrForkNotFound
	}

	return header, fork, nil
}

// WatchSyncWithPeer subscribes and adds peer's latest block
func (s *Syncer) WatchSyncWithPeer(p *SyncPeer, handler func(b *types.Block) bool) {
	// purge from the cache of broadcasted blocks all the ones we have written so far
	header := s.blockchain.Header()
	p.purgeBlocks(header.Hash)

	// listen and enqueue the messages
	for {
		if p.IsClosed() {
			s.logger.Info("Connection to a peer has closed already", "id", p.peer)

			break
		}

		b, err := p.popBlock(popTimeout)
		if err != nil {
			s.logSyncPeerPopBlockError(err, p)

			break
		}

		if err := s.blockchain.WriteBlock(b); err != nil {
			s.logger.Error("failed to write block", "err", err)

			break
		}

		if handler(b) {
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

// BulkSyncWithPeer finds common ancestor with a peer and syncs block until latest block
func (s *Syncer) BulkSyncWithPeer(p *SyncPeer, newBlockHandler func(block *types.Block)) error {
	// find the common ancestor
	ancestor, fork, err := s.findCommonAncestor(p.client, p.status)
	if err != nil {
		return err
	}

	// find in batches
	s.logger.Debug("fork found", "ancestor", ancestor.Number)

	startBlock := fork

	var lastTarget uint64

	// Create a blockchain subscription for the sync progression and start tracking
	s.syncProgression.StartProgression(startBlock.Number, s.blockchain.SubscribeEvents())

	// Stop monitoring the sync progression upon exit
	defer s.syncProgression.StopProgression()

	// sync up to the current known header
	for {
		// update target
		target := p.status.Number

		s.syncProgression.UpdateHighestProgression(target)

		if target == lastTarget {
			// there are no more changes to pull for now
			break
		}

		for {
			s.logger.Debug("sync up to block", "from", startBlock.Number, "to", target)

			// start to synchronize with it
			sk := &skeleton{
				span: 10,
				num:  5,
			}

			if err := sk.build(p.client, startBlock.Hash); err != nil {
				return fmt.Errorf("failed to build skeleton: %w", err)
			}

			// fill skeleton
			for indx := range sk.slots {
				sk.fillSlot(uint64(indx), p.client) //nolint
			}

			// sync the data
			for _, slot := range sk.slots {
				for _, block := range slot.blocks {
					if err := s.blockchain.WriteBlock(block); err != nil {
						return fmt.Errorf("failed to write bulk sync blocks: %w", err)
					}

					newBlockHandler(block)
				}
			}

			// try to get the next block
			startBlock = sk.LastHeader()

			if startBlock.Number >= target {
				break
			}
		}

		lastTarget = target
	}

	return nil
}

func getHeader(clt proto.V1Client, num *uint64, hash *types.Hash) (*types.Header, error) {
	req := &proto.GetHeadersRequest{}
	if num != nil {
		req.Number = int64(*num)
	}

	if hash != nil {
		req.Hash = (*hash).String()
	}

	resp, err := clt.GetHeaders(context.Background(), req)
	if err != nil {
		return nil, err
	}

	if len(resp.Objs) == 0 {
		return nil, nil
	}

	if len(resp.Objs) != 1 {
		return nil, fmt.Errorf("unexpected more than 1 result")
	}

	header := &types.Header{}

	if err := header.UnmarshalRLP(resp.Objs[0].Spec.Value); err != nil {
		return nil, err
	}

	return header, nil
}
