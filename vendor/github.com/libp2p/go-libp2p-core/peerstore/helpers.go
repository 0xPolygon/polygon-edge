package peerstore

import (
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
)

// AddrInfos returns an AddrInfo for each specified peer ID, in-order.
// Deprecated: use github.com/libp2p/go-libp2p/core/peerstore.AddrInfos instead
func AddrInfos(ps Peerstore, peers []peer.ID) []peer.AddrInfo {
	return peerstore.AddrInfos(ps, peers)
}
