package nofork

import (
	"context"

	"github.com/0xPolygon/polygon-edge/network/grpc"
	"github.com/0xPolygon/polygon-edge/syncer/peers"
	"github.com/0xPolygon/polygon-edge/syncer/peers/nofork/proto"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p-core/peer"
)

const (
	SyncPeerServiceLoggerName = "sync-peer-service"
)

type SyncPeerService struct {
	proto.UnimplementedSyncPeerServer

	logger     hclog.Logger
	blockchain Blockchain
	network    Network

	id           string // node ID
	peerStatusCh chan *NoForkPeer
}

func NewSyncPeerService(
	logger hclog.Logger,
	blockchain Blockchain,
	network Network,
) *SyncPeerService {
	return &SyncPeerService{
		logger:       logger.Named(SyncPeerServiceLoggerName),
		blockchain:   blockchain,
		network:      network,
		id:           network.AddrInfo().ID.String(),
		peerStatusCh: make(chan *NoForkPeer, 1),
	}
}

func (s *SyncPeerService) Start() {
	s.setupGRPCServer()
}

func (s *SyncPeerService) setupGRPCServer() {
	grpcStream := grpc.NewGrpcStream()

	proto.RegisterSyncPeerServer(grpcStream.GrpcServer(), s)
	grpcStream.Serve()
	s.network.RegisterProtocol(peers.SyncerProto, grpcStream)
}

// GetBlocks is a gRPC endpoint to return blocks from the specific height via stream
func (s *SyncPeerService) GetBlocks(
	req *proto.GetBlocksRequest,
	stream proto.SyncPeer_GetBlocksServer,
) error {
	// from to latest
	for i := req.From; i <= s.blockchain.Header().Number; i++ {
		block, ok := s.blockchain.GetBlockByNumber(i, true)
		if !ok {
			return peers.ErrorBlockNotFound
		}

		resp := toProtoBlock(block)

		if err := stream.Send(resp); err != nil {
			break
		}
	}

	return nil
}

// GetBlock is a gRPC endpoint to return a block at the specific height
func (s *SyncPeerService) GetBlock(
	ctx context.Context,
	req *proto.GetBlockRequest,
) (*proto.Block, error) {
	block, ok := s.blockchain.GetBlockByNumber(req.Number, true)
	if !ok {
		return nil, peers.ErrorBlockNotFound
	}

	resp := toProtoBlock(block)

	return resp, nil
}

// GetStatus is a gRPC endpoint to return the latest block number as a node status
func (s *SyncPeerService) GetStatus(
	ctx context.Context,
	req *empty.Empty,
) (*proto.Status, error) {
	header := s.blockchain.Header()
	if header == nil {
		return nil, peers.ErrorHeaderNotFound
	}

	return &proto.Status{
		Id:     s.id,
		Number: header.Number,
	}, nil
}

// NotifyStatus is a gRPC endpoint to update peer's status to the local heap
func (s *SyncPeerService) NotifyStatus(
	ctx context.Context,
	req *proto.Status,
) (*empty.Empty, error) {
	s.peerStatusCh <- &NoForkPeer{
		id:       req.Id,
		Number:   req.Number,
		Distance: s.network.GetPeerDistance(peer.ID(req.Id)),
	}

	return &empty.Empty{}, nil
}

func (s *SyncPeerService) PeerStatusCh() <-chan *NoForkPeer {
	return s.peerStatusCh
}

func toProtoBlock(block *types.Block) *proto.Block {
	return &proto.Block{
		Block: block.MarshalRLP(),
	}
}
