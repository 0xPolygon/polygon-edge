package server

import (
	"context"
	"time"

	"github.com/0xPolygon/polygon-sdk/network"
	"github.com/0xPolygon/polygon-sdk/server/proto"
	"github.com/libp2p/go-libp2p-core/peer"
	empty "google.golang.org/protobuf/types/known/emptypb"
)

type systemService struct {
	proto.UnimplementedSystemServer

	s *Server
}

// GetStatus returns the current system status, in the form of:
//
// Network: <chainID>
//
// Current: { Number: <blockNumber>; Hash: <headerHash> }
//
// P2PAddr: <libp2pAddress>
func (s *systemService) GetStatus(ctx context.Context, req *empty.Empty) (*proto.ServerStatus, error) {
	header := s.s.blockchain.Header()

	status := &proto.ServerStatus{
		Network: int64(s.s.chain.Params.ChainID),
		Current: &proto.ServerStatus_Block{
			Number: int64(header.Number),
			Hash:   header.Hash.String(),
		},
		P2PAddr: network.AddrInfoToString(s.s.network.AddrInfo()),
	}
	return status, nil
}

// Subscribe implements the blockchain event subscription service
func (s *systemService) Subscribe(req *empty.Empty, stream proto.System_SubscribeServer) error {
	sub := s.s.blockchain.SubscribeEvents()

	for {
		evnt := sub.GetEvent()
		if evnt == nil {
			break
		}
		pEvent := &proto.BlockchainEvent{
			Added:   []*proto.BlockchainEvent_Header{},
			Removed: []*proto.BlockchainEvent_Header{},
		}
		for _, h := range evnt.NewChain {
			pEvent.Added = append(
				pEvent.Added,
				&proto.BlockchainEvent_Header{Hash: h.Hash.String(), Number: int64(h.Number)},
			)
		}
		for _, h := range evnt.OldChain {
			pEvent.Removed = append(
				pEvent.Removed,
				&proto.BlockchainEvent_Header{Hash: h.Hash.String(), Number: int64(h.Number)},
			)
		}
		err := stream.Send(pEvent)

		if err != nil {
			break
		}
	}

	sub.Close()

	return nil
}

// PeersAdd implements the 'peers add' operator service
func (s *systemService) PeersAdd(ctx context.Context, req *proto.PeersAddRequest) (*empty.Empty, error) {
	dur := time.Duration(0)
	if req.Blocked {
		dur = network.DefaultJoinTimeout
	}

	err := s.s.Join(req.Id, dur)

	return &empty.Empty{}, err
}

// PeersStatus implements the 'peers status' operator service
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

// getPeer returns a specific proto.Peer using the peer ID
func (s *systemService) getPeer(id peer.ID) (*proto.Peer, error) {
	protocols, err := s.s.network.GetProtocols(id)
	if err != nil {
		return nil, err
	}
	info := s.s.network.GetPeerInfo(id)

	addrs := []string{}
	for _, addr := range info.Addrs {
		addrs = append(addrs, addr.String())
	}

	peer := &proto.Peer{
		Id:        id.String(),
		Protocols: protocols,
		Addrs:     addrs,
	}

	return peer, nil
}

// PeersList implements the 'peers list' operator service
func (s *systemService) PeersList(
	ctx context.Context,
	req *empty.Empty,
) (*proto.PeersListResponse, error) {
	resp := &proto.PeersListResponse{
		Peers: []*proto.Peer{},
	}

	peers := s.s.network.Peers()
	for _, p := range peers {
		peer, err := s.getPeer(p.Info.ID)
		if err != nil {
			return nil, err
		}

		resp.Peers = append(resp.Peers, peer)
	}

	return resp, nil
}
