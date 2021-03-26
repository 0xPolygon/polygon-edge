package minimal

import (
	"context"

	"github.com/0xPolygon/minimal/minimal/proto"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/libp2p/go-libp2p-core/peer"
)

type systemService struct {
	proto.UnimplementedSystemServer

	s *Server
}

func (s *systemService) GetStatus(ctx context.Context, req *empty.Empty) (*proto.ServerStatus, error) {
	header := s.s.blockchain.Header()

	status := &proto.ServerStatus{
		Network: int64(s.s.chain.Params.ChainID),
		Current: &proto.ServerStatus_Block{
			Number: int64(header.Number),
			Hash:   header.Hash.String(),
		},
		// P2PAddr: AddrInfoToString(s.s.AddrInfo()),
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

func (s *systemService) PeersAdd(ctx context.Context, req *proto.PeersAddRequest) (*empty.Empty, error) {
	err := s.s.Join(req.Id)
	return &empty.Empty{}, err
}

func (s *systemService) PeersStatus(ctx context.Context, req *proto.PeersStatusRequest) (*proto.Peer, error) {
	peerID, err := peer.Decode(req.Id)
	if err != nil {
		return nil, err
	}
	peer, err := s.getPeer(peerID)
	if err != nil {
		return nil, err
	}
	return peer, nil
}

func (s *systemService) getPeer(id peer.ID) (*proto.Peer, error) {
	/*
		protocols, err := s.s.host.Peerstore().GetProtocols(id)
		if err != nil {
			return nil, err
		}
		info := s.s.host.Peerstore().PeerInfo(id)
		addrs := []string{}
		for _, addr := range info.Addrs {
			addrs = append(addrs, addr.String())
		}
		peer := &proto.Peer{
			Id:        id.String(),
			Protocols: protocols,
			Addrs:     addrs,
		}
	*/
	return nil, nil
}

func (s *systemService) PeersList(ctx context.Context, req *empty.Empty) (*proto.PeersListResponse, error) {
	resp := &proto.PeersListResponse{
		Peers: []*proto.Peer{},
	}
	/*
		ids := s.s.host.Peerstore().Peers()
		for _, id := range ids {
			peer, err := s.getPeer(id)
			if err != nil {
				return nil, err
			}
			resp.Peers = append(resp.Peers, peer)
		}
	*/
	return resp, nil
}
