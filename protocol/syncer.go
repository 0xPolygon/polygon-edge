package protocol

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/big"
	"sync"

	"github.com/0xPolygon/minimal/blockchain"
	"github.com/0xPolygon/minimal/network"
	libp2pGrpc "github.com/0xPolygon/minimal/network/grpc"
	"github.com/0xPolygon/minimal/protocol/proto"
	"github.com/0xPolygon/minimal/types"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p-core/peer"
	"google.golang.org/grpc"
)

const maxEnqueueSize = 50

// syncPeer is a representation of the peer the node is syncing with
type syncPeer struct {
	peer   peer.ID
	client proto.V1Client
	status *Status

	// current status
	number uint64
	hash   types.Hash

	enqueueLock sync.Mutex
	enqueue     []*types.Block
	enqueueCh   chan struct{}
}

// Number returns the latest peer block height
func (s *syncPeer) Number() uint64 {
	return s.number
}

// purgeBlocks purges the cache of broadcasted blocks the node has written so far
// from the syncPeer
func (s *syncPeer) purgeBlocks(lastSeen types.Hash) {
	s.enqueueLock.Lock()
	defer s.enqueueLock.Unlock()

	indx := -1
	for i, b := range s.enqueue {
		if b.Hash() == lastSeen {
			indx = i
		}
	}
	if indx != -1 {
		s.enqueue = s.enqueue[indx:]
	}
}

// popBlock pops a block from the block queue [BLOCKING]
func (s *syncPeer) popBlock() (b *types.Block) {
	for {
		s.enqueueLock.Lock()
		if len(s.enqueue) != 0 {
			b, s.enqueue = s.enqueue[0], s.enqueue[1:]
			s.enqueueLock.Unlock()
			return
		}

		s.enqueueLock.Unlock()
		<-s.enqueueCh
	}
}

// appendBlock adds a new block to the block queue
func (s *syncPeer) appendBlock(b *types.Block) {
	s.enqueueLock.Lock()
	defer s.enqueueLock.Unlock()

	if len(s.enqueue) == maxEnqueueSize {
		// pop first element
		s.enqueue = s.enqueue[1:]
	}
	// append to the end
	s.enqueue = append(s.enqueue, b)

	// update the number and hash
	s.number = b.Number()
	s.hash = b.Hash()

	select {
	case s.enqueueCh <- struct{}{}:
	default:
	}
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

	peers map[peer.ID]*syncPeer // TODO: Remove

	serviceV1 *serviceV1
	stopCh    chan struct{}

	status     *Status
	statusLock sync.Mutex

	server *network.Server
}

// NewSyncer creates a new Syncer instance
func NewSyncer(logger hclog.Logger, server *network.Server, blockchain blockchainShim) *Syncer {
	s := &Syncer{
		logger:     logger.Named("syncer"),
		peers:      map[peer.ID]*syncPeer{},
		stopCh:     make(chan struct{}),
		blockchain: blockchain,
		server:     server,
	}

	return s
}

// syncCurrentStatus taps into the blockchain event steam and updates the Syncer.status field
func (s *Syncer) syncCurrentStatus() {
	// Get the current status of the syncer
	currentHeader := s.blockchain.Header()
	diff, _ := s.blockchain.GetTD(currentHeader.Hash)

	s.status = &Status{
		Hash:       currentHeader.Hash,
		Number:     currentHeader.Number,
		Difficulty: diff,
	}

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

	p, ok := s.peers[peerID]
	if ok {
		p.appendBlock(b)
	}
}

// Broadcast broadcasts a block to all peers
func (s *Syncer) Broadcast(b *types.Block) {
	// diff is number in ibft
	diff := new(big.Int).SetUint64(b.Number())

	// broadcast the new block to all the peers
	req := &proto.NotifyReq{
		Status: &proto.V1Status{
			Hash:       b.Hash().String(),
			Number:     b.Number(),
			Difficulty: diff.String(),
		},
		Raw: &any.Any{
			Value: b.MarshalRLP(),
		},
	}
	for _, p := range s.peers {
		if _, err := p.client.Notify(context.Background(), req); err != nil {
			s.logger.Error("failed to notify", "err", err)
		}
	}
}

// Start starts the syncer protocol
func (s *Syncer) Start() {
	s.serviceV1 = &serviceV1{syncer: s, logger: hclog.NewNullLogger(), store: s.blockchain}

	// Run the blockchain event listener loop
	go s.syncCurrentStatus()

	// Register the grpc protocol for syncer
	grpcStream := libp2pGrpc.NewGrpcStream()
	proto.RegisterV1Server(grpcStream.GrpcServer(), s.serviceV1)
	grpcStream.Serve()

	s.server.Register(syncerV1, grpcStream)

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

			if evnt.Type != network.PeerEventConnected {
				continue
			}

			stream, err := s.server.NewStream(syncerV1, evnt.PeerID)
			if err != nil {
				s.logger.Error("failed to open a stream", "err", err)
				continue
			}
			if err := s.HandleUser(evnt.PeerID, libp2pGrpc.WrapClient(stream)); err != nil {
				s.logger.Error("failed to handle user", "err", err)
			}
		}
	}()
}

// BestPeer returns the best peer by difficulty (if any)
func (s *Syncer) BestPeer() *syncPeer {
	var bestPeer *syncPeer
	var bestTd *big.Int

	for _, p := range s.peers {
		status := p.status
		if bestPeer == nil || status.Difficulty.Cmp(bestTd) > 0 {
			bestPeer, bestTd = p, status.Difficulty
		}
	}
	if bestPeer == nil {
		return nil
	}
	curDiff := s.blockchain.CurrentTD()
	if bestTd.Cmp(curDiff) <= 0 {
		return nil
	}
	return bestPeer
}

// HandleUser is a helper method that is used to handle new user connections within the Syncer
func (s *Syncer) HandleUser(peerID peer.ID, conn *grpc.ClientConn) error {
	// watch for changes of the other node first
	clt := proto.NewV1Client(conn)

	rawStatus, err := clt.GetCurrent(context.Background(), &empty.Empty{})
	if err != nil {
		return err
	}
	status, err := statusFromProto(rawStatus)
	if err != nil {
		return err
	}
	s.peers[peerID] = &syncPeer{
		peer:      peerID,
		client:    clt,
		status:    status,
		enqueueCh: make(chan struct{}),
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
				return nil, nil, fmt.Errorf("failed to read local genesis")
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
					return nil, nil, errors.New("genesis does not match?")
				}
				max = m - 1
			}
		}
	}

	if header == nil {
		return nil, nil, errors.New("header is nil")
	}

	// get the block fork
	forkNum := header.Number + 1
	fork, err := getHeader(clt, &forkNum, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get fork at num %d", header.Number)
	}
	if fork == nil {
		return nil, nil, errors.New("fork not found")
	}

	return header, fork, nil
}

func (s *Syncer) WatchSyncWithPeer(p *syncPeer, handler func(b *types.Block) bool) {
	// purge from the cache of broadcasted blocks all the ones we have written so far
	header := s.blockchain.Header()
	p.purgeBlocks(header.Hash)

	// listen and enqueue the messages
	for {
		b := p.popBlock()

		if err := s.blockchain.WriteBlocks([]*types.Block{b}); err != nil {
			s.logger.Error("failed to write block", "err", err)
			break
		}
		if !handler(b) {
			break
		}
	}
}

func (s *Syncer) BulkSyncWithPeer(p *syncPeer) error {
	// find the common ancestor
	ancestor, fork, err := s.findCommonAncestor(p.client, p.status)
	if err != nil {
		return err
	}

	// find in batches
	s.logger.Debug("fork found", "ancestor", ancestor.Number)

	startBlock := fork

	var lastTarget uint64

	// sync up to the current known header
	for {
		// update target
		target := p.status.Number
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
				return fmt.Errorf("failed to build skeleton: %v", err)
			}

			// fill skeleton
			for indx := range sk.slots {
				sk.fillSlot(uint64(indx), p.client) //nolint
			}

			// sync the data
			for _, slot := range sk.slots {
				if err := s.blockchain.WriteBlocks(slot.blocks); err != nil {
					return fmt.Errorf("failed to write bulk sync blocks: %v", err)
				}
			}

			// try to get the next block
			startBlock = sk.LastHeader()

			if startBlock.Number >= uint64(target) {
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
