package syncer

import (
	"context"
	"errors"

	"github.com/0xPolygon/polygon-edge/network/grpc"
	"github.com/0xPolygon/polygon-edge/syncer/proto"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/golang/protobuf/ptypes/empty"
)

var (
	ErrBlockNotFound = errors.New("block not found")
)

type syncPeerService struct {
	proto.UnimplementedSyncPeerServer

	blockchain Blockchain
	network    Network

	id string // node ID
}

func NewSyncPeerService(
	network Network,
	blockchain Blockchain,
) SyncPeerService {
	return &syncPeerService{
		blockchain: blockchain,
		network:    network,
		id:         network.AddrInfo().ID.String(),
	}
}

func (s *syncPeerService) Start() {
	s.setupGRPCServer()
}

func (s *syncPeerService) setupGRPCServer() {
	grpcStream := grpc.NewGrpcStream()

	proto.RegisterSyncPeerServer(grpcStream.GrpcServer(), s)
	grpcStream.Serve()
	s.network.RegisterProtocol(SyncerProto, grpcStream)
}

// GetBlocks is a gRPC endpoint to return blocks from the specific height via stream
func (s *syncPeerService) GetBlocks(
	req *proto.GetBlocksRequest,
	stream proto.SyncPeer_GetBlocksServer,
) error {
	// from to latest
	for i := req.From; i <= s.blockchain.Header().Number; i++ {
		block, ok := s.blockchain.GetBlockByNumber(i, true)
		if !ok {
			return ErrBlockNotFound
		}

		resp := toProtoBlock(block)

		if err := stream.Send(resp); err != nil {
			break
		}
	}

	return nil
}

// GetBlock is a gRPC endpoint to return a block at the specific height
func (s *syncPeerService) GetBlock(
	ctx context.Context,
	req *proto.GetBlockRequest,
) (*proto.Block, error) {
	block, ok := s.blockchain.GetBlockByNumber(req.Number, true)
	if !ok {
		return nil, ErrBlockNotFound
	}

	resp := toProtoBlock(block)

	return resp, nil
}

// GetStatus is a gRPC endpoint to return the latest block number as a node status
func (s *syncPeerService) GetStatus(
	ctx context.Context,
	req *empty.Empty,
) (*proto.SyncPeerStatus, error) {
	var number uint64
	if header := s.blockchain.Header(); header != nil {
		number = header.Number
	}

	return &proto.SyncPeerStatus{
		Number: number,
	}, nil
}

func toProtoBlock(block *types.Block) *proto.Block {
	return &proto.Block{
		Block: block.MarshalRLP(),
	}
}
