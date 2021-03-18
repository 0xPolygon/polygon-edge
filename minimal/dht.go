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

	go s.handlePeerChanged()

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

func (s *Server) handlePeerChanged() {
	for {
		select {
		case <-s.peerAddedCh:
		case <-s.peerRemovedCh:
		}

		pc, err := s.dht.GetClosestPeers(context.Background(), string(s.host.ID()))
		if err == nil {
			for peer := range pc {
				peerInfo := s.host.Peerstore().PeerInfo(peer)
				addr0 := AddrInfoToString(&peerInfo)
				s.Join(addr0)
			}
		}
	}
}
