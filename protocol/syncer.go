package protocol

import (
	"context"
	"fmt"
	"math"
	"math/big"
	"sync"
	"time"

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

type syncPeer struct {
	peer   peer.ID
	client proto.V1Client
	status *proto.V1Status

	// current status
	number uint64
	hash   types.Hash

	enqueueLock sync.Mutex
	enqueue     []*types.Block
	enqueueCh   chan struct{}
}

func (s *syncPeer) Number() uint64 {
	return s.number
}

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

func (s *syncPeer) appendBlock(b *types.Block) {
	s.enqueueLock.Lock()
	defer s.enqueueLock.Unlock()

	if len(s.enqueue) == maxEnqueueSize {
		// pop first element
		s.enqueue = s.enqueue[1:]
	}
	// append to the end
	s.enqueue = append(s.enqueue, b)

	// update the number (TODO LOCK THIS)
	s.number = b.Number()
	s.hash = b.Hash()

	select {
	case s.enqueueCh <- struct{}{}:
	default:
	}
}

type headUpdate struct {
	peer   string
	status *proto.V1Status
}

type status int

const (
	statusSync status = iota
	statusWatch
)

// Syncer is a sync protocol
type Syncer struct {
	logger     hclog.Logger
	blockchain blockchainShim

	peersLock sync.Mutex            // TODO: Remove
	peers     map[peer.ID]*syncPeer // TODO: Remove
	newPeerCh chan struct{}         // TODO: Remove

	// status of the syncing process
	status status

	serviceV1 *serviceV1
	stopCh    chan struct{}

	// TODO: We use the new network.Server
	server *network.Server

	SyncNotifyCh chan bool

	// id of the current target
	target string
}

func NewSyncer(logger hclog.Logger, server *network.Server, blockchain blockchainShim) *Syncer {
	s := &Syncer{
		logger:     logger.Named("syncer"),
		peers:      map[peer.ID]*syncPeer{},
		stopCh:     make(chan struct{}),
		blockchain: blockchain,
		newPeerCh:  make(chan struct{}),
		status:     statusSync,
		server:     server,
	}
	return s
}

func (s *Syncer) notifySync(synced bool) {
	if s.SyncNotifyCh != nil {
		s.SyncNotifyCh <- synced
	}
}

const syncerV1 = "/syncer/0.1"

func (s *Syncer) enqueueBlock(peerID peer.ID, b *types.Block) {
	s.logger.Debug("enqueue block", "peer", peerID, "number", b.Number(), "hash", b.Hash())

	p, ok := s.peers[peerID]
	if ok {
		p.appendBlock(b)
	}
}

func (s *Syncer) Broadcast(b *types.Block) {
	// broadcast the new block to all the peers
	req := &proto.NotifyReq{
		Hash:   b.Hash().String(),
		Number: int64(b.Number()),
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

func (s *Syncer) Start() {
	s.serviceV1 = &serviceV1{syncer: s, logger: hclog.NewNullLogger(), store: s.blockchain, subs: s.blockchain.SubscribeEvents()}

	// register the grpc protocol for syncer
	grpc := libp2pGrpc.NewGrpcStream()
	proto.RegisterV1Server(grpc.GrpcServer(), s.serviceV1)

	s.server.Register(syncerV1, grpc)

	updateCh, err := s.server.SubscribeCh()
	if err != nil {
		panic(err)
	}

	go func() {
		for {
			evnt := <-updateCh
			if evnt.Type != network.PeerEventConnected {
				continue
			}

			stream, err := s.server.NewStream(syncerV1, evnt.PeerID)
			if err != nil {
				panic(err)
			}
			s.HandleUser(evnt.PeerID, libp2pGrpc.WrapClient(stream))
		}
	}()
	go s.serviceV1.start()
	//go s.syncPeerImpl()
}

func (s *Syncer) BestPeer() *syncPeer {
	var bestPeer *syncPeer
	var bestTd *big.Int

	for _, p := range s.peers {
		diff, err := types.ParseUint256orHex(&p.status.Difficulty)
		if err != nil {
			panic(err)
		}
		if bestPeer == nil || diff.Cmp(bestTd) > 0 {
			bestPeer, bestTd = p, diff
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

func (s *Syncer) syncPeerImpl() {
	for {
		select {
		case <-s.newPeerCh:
		case <-time.After(10 * time.Second):
		case <-s.stopCh:
			return
		}

		// check who is the best peer
		best := s.BestPeer()
		if best == nil {
			continue
		}

		// s.syncWithPeer(best)
	}
}

func (s *Syncer) HandleUser(peerID peer.ID, conn *grpc.ClientConn) {
	fmt.Println("- new user -")

	// watch for changes of the other node first
	clt := proto.NewV1Client(conn)

	status, err := clt.GetCurrent(context.Background(), &empty.Empty{})
	if err != nil {
		panic(err)
	}

	peer := &syncPeer{
		peer:      peerID,
		client:    clt,
		status:    status,
		enqueueCh: make(chan struct{}),
	}
	s.peers[peerID] = peer

	select {
	case s.newPeerCh <- struct{}{}:
	default:
	}

	/*
		stream, err := clt.Watch(context.Background(), &empty.Empty{})
		if err != nil {
			panic(err)
		}

		go func() {
			for {
				status, err := stream.Recv()
				if err != nil {
					panic(err)
				}

				fmt.Println("-- status --")
				fmt.Println(status)

				// retrieve the block

				hash := types.StringToHash(status.Hash)
				header, err := getHeader(clt, nil, &hash)
				if err != nil {
					panic(err)
				}
				block := &types.Block{
					Header: header,
				}
				if err := s.blockchain.WriteBlocks([]*types.Block{block}); err != nil {
					s.logger.Error("failed to write block", "err", err)
				}
			}
		}()
	*/
}

func (s *Syncer) findCommonAncestor(clt proto.V1Client, status *proto.V1Status) (*types.Header, *types.Header, error) {
	h := s.blockchain.Header()

	min := uint64(0) // genesis
	max := h.Number

	targetHeight := uint64(status.Number)

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

		/*
			fmt.Println("- req -")
			fmt.Println(min, max, m)
			time.Sleep(1 * time.Second)
		*/

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
					return nil, nil, fmt.Errorf("genesis does not match?")
				}
				max = m - 1
			}
		}
	}

	// get the block fork
	forkNum := header.Number + 1
	fork, err := getHeader(clt, &forkNum, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get fork at num %d", header.Number)
	}
	if fork == nil {
		panic("fork not found")
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

		fmt.Println("--- bb ---")
		fmt.Println(b)

		if err := s.blockchain.WriteBlocks([]*types.Block{b}); err != nil {
			panic(err)
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

	var lastTarget int64

	// sync up to the current known header for the
	for {
		// update target
		target := p.status.Number
		if target == lastTarget {
			// there are no more changes to pull for now
			break
		}

		for {
			s.logger.Debug("sync up to block", "from", startBlock.Number, "to", target)

			// start to syncronize with it
			sk := &skeleton{
				span: 10,
				num:  5,
			}
			if err := sk.build(p.client, startBlock.Hash); err != nil {
				panic(err)
			}

			// fill skeleton
			for indx := range sk.slots {
				sk.fillSlot(uint64(indx), p.client)
			}

			// sync the data
			for _, slot := range sk.slots {
				for _, b := range slot.blocks {
					fmt.Printf("Block %d %s\n", b.Number(), b.Hash().String())
				}
				if err := s.blockchain.WriteBlocks(slot.blocks); err != nil {
					panic(err)
				}
			}

			// try to get the next block
			startBlock = sk.LastHeader()

			/*
				// validate that we have written to the correct place
				lastWrittenHeader := s.blockchain.Header()
				if startBlock.Hash != lastWrittenHeader.Hash {
					// this might mean we are writting to a fork??
					panic("bad")
				}

				fmt.Println("- start block -")
				fmt.Println(startBlock.Number)
				fmt.Println(target)
			*/

			if startBlock.Number >= uint64(target) {
				break
			}
		}

		lastTarget = target
	}

	fmt.Println("___ BATCH SYNC DONE ___")

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
