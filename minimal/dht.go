package minimal

import (
	"context"

	"github.com/ipfs/go-ipns"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	record "github.com/libp2p/go-libp2p-record"
)

func (s *Server) setupDHT(ctx context.Context, host host.Host) error {
	nsValidator := record.NamespacedValidator{}
	nsValidator["ipns"] = ipns.Validator{}
	nsValidator["pk"] = record.PublicKeyValidator{}

	d, err := dht.New(ctx, host, dht.Mode(dht.ModeServer), dht.Validator(nsValidator), dht.BootstrapPeers())
	if err != nil {
		return err
	} else if err = d.Bootstrap(ctx); err != nil {
		return err
	}

	s.dht = d
	s.dht.RoutingTable().PeerAdded = s.peerAdded
	s.dht.RoutingTable().PeerRemoved = s.peerRemoved

	go s.handlePeerChanged(ctx)

	return nil
}

func (s *Server) peerAdded(p peer.ID) {
	s.logger.Info("Peer added", "peer", p.String())
	select {
	case s.peerAddedCh <- struct{}{}:
	default:
	}
}

func (s *Server) peerRemoved(p peer.ID) {
	s.logger.Info("Peer removed", "peer", p.String())

	select {
	case s.peerRemovedCh <- struct{}{}:
	default:
	}
}

func (s *Server) handlePeerChanged(ctx context.Context) {
	for {
		select {
		case <-s.peerAddedCh:
		case <-s.peerRemovedCh:
		case <-ctx.Done():
			return
		}
		s.addBestPeer()
	}
}

func (s *Server) addBestPeer() {
	addr0, err := s.getBestPeerAddr()
	if err != nil {
		s.logger.Error("Failed to get best peer", "error", err.Error())
	}
	if addr0 != nil {
		s.Join(*addr0)
	}
}

func (s *Server) getBestPeerAddr() (*string, error) {
	pc, err := s.dht.GetClosestPeers(context.Background(), string(s.host.ID()))
	if err != nil {
		return nil, err
	}

	peers := []peer.ID{}
	for p := range pc {
		peers = append(peers, p)
	}

	for _, peer := range peers {
		peerInfo := s.host.Peerstore().PeerInfo(peer)
		addr0 := AddrInfoToString(&peerInfo)
		if !s.getPeerJoined(addr0) {
			return &addr0, nil
		}
	}
	return nil, nil
}
