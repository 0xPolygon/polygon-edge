package syncer

import (
	"context"
	"errors"
	"sync/atomic"

	"github.com/0xPolygon/polygon-edge/syncer/proto"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/golang/protobuf/ptypes/empty"
)

const (
	SyncerProto = "/syncer/0.2"
)

var (
	ErrorBlockNotFound = errors.New("block not found")
)

type SyncerService interface {
	proto.SyncerServer

	SetStatus(number uint64)
}

type UpdatePeerStatusEvent struct {
	// ID?

	Number uint64
}

type syncerService struct {
	proto.UnimplementedSyncerServer

	blockchain              Blockchain
	number                  uint64
	updatePeerStatusHandler func(*UpdatePeerStatusEvent)
}

func NewSyncerService(blockchain Blockchain, updatePeerStatusHandler func(*UpdatePeerStatusEvent)) SyncerService {
	return &syncerService{
		blockchain:              blockchain,
		number:                  0,
		updatePeerStatusHandler: updatePeerStatusHandler,
	}
}

func (s *syncerService) SetStatus(number uint64) {
	atomic.StoreUint64(&s.number, number)
}

func (s *syncerService) GetBlocks(
	req *proto.GetBlocksRequest,
	stream proto.Syncer_GetBlocksServer,
) error {
	// from to latest
	for i := req.From; i <= s.blockchain.Header().Number; i++ {
		block, ok := s.blockchain.GetBlockByNumber(i, true)
		if !ok {
			return ErrorBlockNotFound
		}

		resp := toProtoBlock(block)

		if err := stream.Send(resp); err != nil {
			break
		}
	}

	return nil
}

func (s *syncerService) GetBlock(
	ctx context.Context,
	req *proto.GetBlockRequest,
) (*proto.Block, error) {
	block, ok := s.blockchain.GetBlockByNumber(req.Number, true)
	if !ok {
		return nil, ErrorBlockNotFound
	}

	resp := toProtoBlock(block)

	return resp, nil
}

func (s *syncerService) GetStatus(
	ctx context.Context,
	req *empty.Empty,
) (*proto.Status, error) {
	status := atomic.LoadUint64(&s.number)

	return &proto.Status{
		Number: status,
	}, nil
}

func (s *syncerService) NotifyStatus(
	ctx context.Context,
	req *proto.Status,
) (*empty.Empty, error) {
	if s.updatePeerStatusHandler != nil {
		s.updatePeerStatusHandler(&UpdatePeerStatusEvent{
			Number: req.Number,
		})
	}

	return &empty.Empty{}, nil
}

func toProtoBlock(block *types.Block) *proto.Block {
	return &proto.Block{
		Block: block.MarshalRLP(),
	}
}
