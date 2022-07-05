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
}

func NewSyncPeerService(
	network Network,
	blockchain Blockchain,
) SyncPeerService {
	return &syncPeerService{
		blockchain: blockchain,
		network:    network,
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

		// if client closes stream, context.Canceled is given
		if err := stream.Send(resp); err != nil {

			break
		}
	}

	return nil
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
