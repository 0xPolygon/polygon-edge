package syncer

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/0xPolygon/polygon-edge/blockchain"
	"github.com/0xPolygon/polygon-edge/helper/progress"
	"github.com/0xPolygon/polygon-edge/network"
	"github.com/0xPolygon/polygon-edge/network/event"
	"github.com/0xPolygon/polygon-edge/network/grpc"
	"github.com/0xPolygon/polygon-edge/syncer/proto"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	libp2pNetwork "github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
)

const (
	LoggerName = "syncer"
)

var (
	ErrPopTimeout       = errors.New("timeout")
	ErrNoPeersConnected = errors.New("no peers connected")
)

type Syncer interface {
	Start()
	GetSyncProgression() *progress.Progression
	HasSyncPeer() bool
	BulkSync(context.Context) error
	WatchSync(context.Context, func(*types.Block) bool, time.Duration) error
}

type Network interface {
	// Register gRPC service
	RegisterProtocol(string, network.Protocol)
	// Get current connected peers
	Peers() []*network.PeerConnInfo
	// Subscribe peer added/removed events
	SubscribeCh() (<-chan *event.PeerEvent, error)
	// Get distance between node and peer
	GetPeerDistance(peer.ID) *big.Int
	//
	NewStream(string, peer.ID) (libp2pNetwork.Stream, error)
}

type Blockchain interface {
	// Subscribe new block event
	SubscribeEvents() blockchain.Subscription
	// Get latest header
	Header() *types.Header
	// Get block from chain
	GetBlockByNumber(uint64, bool) (*types.Block, bool)
	// Verify fetched block
	VerifyFinalizedBlock(*types.Block) error
	// Write block to chain
	WriteBlock(*types.Block) error
}

type Progression interface {
	StartProgression(startingBlock uint64, subscription blockchain.Subscription)
	UpdateHighestProgression(highestBlock uint64)
	StopProgression()
	GetProgression() *progress.Progression
}

type syncer struct {
	logger       hclog.Logger
	network      Network
	blockchain   Blockchain
	progression  Progression
	service      SyncerService
	peerHeap     PeerHeap
	syncNewBlock chan struct{}
}

func NewSyncer(logger hclog.Logger, network Network, blockchain Blockchain) Syncer {
	return &syncer{
		logger:       logger.Named(LoggerName),
		network:      network,
		blockchain:   blockchain,
		syncNewBlock: make(chan struct{}),
	}
}

func (s *syncer) Start() {
	s.setupGRPCService()
	s.setupPeers()
}

func (s *syncer) setupGRPCService() {
	s.service = NewSyncerService(s.blockchain, s.updatePeerStatus)

	grpcStream := grpc.NewGrpcStream()
	proto.RegisterSyncerServer(grpcStream.GrpcServer(), s.service)
	grpcStream.Serve()
	s.network.RegisterProtocol(SyncerProto, grpcStream)
}

func (s *syncer) setupPeers() {
	s.peerHeap = newPeerHeap(nil)
}

func (s *syncer) updatePeerStatus(e *UpdatePeerStatusEvent) {
	// TODO
}

func (s *syncer) GetSyncProgression() *progress.Progression {
	// TODO
	return nil
}

// HasSyncPeer returns whether syncer has the peer to syncs blocks
// return false if syncer has no peer whose latest block height doesn't exceed local height
func (s *syncer) HasSyncPeer() bool {
	// TODO
	return false
}

func (s *syncer) BulkSync(context.Context) error {
	// pick one best peer

	// open stream and subscribe blocks
	// verify and write blocks

	return nil
}

func (s *syncer) WatchSync(ctx context.Context, callback func(*types.Block) bool, timeout time.Duration) error {
	timeoutCh := time.After(timeout * 3)
	localLatest := uint64(0)
	isValidator := false
	for {
		//wait for a new block event
		select {
		case <-s.syncNewBlock:
		case <-timeoutCh:
			return ErrPopTimeout
		}

		if header := s.blockchain.Header(); header != nil {
			localLatest = header.Number
		}

		// pick one best peer
		bestPeer := s.peerHeap.BestPeer()
		if bestPeer == nil || bestPeer.Number <= localLatest {
			return ErrNoPeersConnected
		}

		// fetch blocks from the peer
		// TODO sync with yoshiki not to have to write double logic
		blocks := []*types.Block{}

		// iterate over blocks
		for _, block := range blocks {

			// verify the block
			if err := s.blockchain.VerifyFinalizedBlock(block); err != nil {
				return fmt.Errorf("unable to verify block, %w", err)
			}

			// write the block
			if err := s.blockchain.WriteBlock(block); err != nil {
				return fmt.Errorf("failed to write block while bulk syncing: %w", err)
			}

			//check if node is validator
			isValidator = callback(block)
		}

		//After syncing all blocks from your best peer, if a validator return from watchSync
		if isValidator {
			return nil
		}
	}
}

func (s *syncer) newSyncerClient(id peer.ID) (proto.SyncerClient, error) {
	stream, err := s.network.NewStream(SyncerProto, id)
	if err != nil {
		return nil, fmt.Errorf("failed to open a stream, err %w", err)
	}

	conn := grpc.WrapClient(stream)

	return proto.NewSyncerClient(conn), nil
}
