package minimal

import (
	"context"
	"sort"

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
	pc, err := s.dht.GetClosestPeers(context.Background(), string(s.host.ID()))
	if err == nil {
		peers := []peer.ID{}
		for p := range pc {
			peers = append(peers, p)
		}
		s.sortPeersByLatency(peers)

		for _, peer := range peers {
			peerInfo := s.host.Peerstore().PeerInfo(peer)
			addr0 := AddrInfoToString(&peerInfo)
			if !s.getPeerJoined(addr0) {
				s.Join(addr0)
				return
			}
		}
	}
}

func (s *Server) sortPeersByLatency(peers []peer.ID) {
	peerToBw := s.bwc.GetBandwidthByPeer()
	sort.SliceStable(peers, func(i, j int) bool {
		p1, p2 := peers[i], peers[j]
		bw1, bw2 := peerToBw[p1], peerToBw[p2]
		if bw1.TotalIn == 0 && bw2.TotalIn == 0 {
			return i < j
		}
		if bw1.TotalIn == 0 {
			return false
		}
		if bw2.TotalIn == 0 {
			return true
		}
		return bw1.RateIn > bw2.RateIn
	})
}
