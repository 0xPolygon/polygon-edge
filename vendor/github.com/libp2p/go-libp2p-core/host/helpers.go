package host

import (
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
)

// InfoFromHost returns a peer.AddrInfo struct with the Host's ID and all of its Addrs.
// Deprecated: use github.com/libp2p/go-libp2p/core/host.InfoFromHost instead
func InfoFromHost(h Host) *peer.AddrInfo {
	return host.InfoFromHost(h)
}
