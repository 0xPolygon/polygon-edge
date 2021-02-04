package minimal

import (
	"context"

	"github.com/0xPolygon/minimal/minimal/proto"
	"github.com/golang/protobuf/ptypes/empty"
)

type systemService struct {
	s *Server
}

func (s *systemService) GetStatus(ctx context.Context, req *empty.Empty) (*proto.Status, error) {
	header, _ := s.s.blockchain.Header()

	status := &proto.Status{
		Network: int64(s.s.chain.Params.ChainID),
		Current: &proto.Status_Block{
			Number: int64(header.Number),
			Hash:   header.Hash.String(),
		},
		P2PAddr: AddrInfoToString(s.s.AddrInfo()),
	}
	return status, nil
}

func (s *systemService) Subscribe(req *empty.Empty, stream proto.System_SubscribeServer) error {
	sub := s.s.blockchain.SubscribeEvents()

	for {
		evnt := sub.GetEvent()
		pEvent := &proto.BlockchainEvent{
			Added:   []*proto.BlockchainEvent_Header{},
			Removed: []*proto.BlockchainEvent_Header{},
		}
		for _, h := range evnt.NewChain {
			pEvent.Added = append(pEvent.Added, &proto.BlockchainEvent_Header{Hash: h.Hash.String(), Number: int64(h.Number)})
		}
		for _, h := range evnt.OldChain {
			pEvent.Removed = append(pEvent.Removed, &proto.BlockchainEvent_Header{Hash: h.Hash.String(), Number: int64(h.Number)})
		}
		err := stream.Send(pEvent)
		if err != nil {
			break
		}
	}

	sub.Close()
	return nil
}
