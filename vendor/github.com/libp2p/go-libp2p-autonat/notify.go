package autonat

import (
	"github.com/libp2p/go-libp2p-core/network"

	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr-net"
)

var _ network.Notifiee = (*AmbientAutoNAT)(nil)

func (as *AmbientAutoNAT) Listen(net network.Network, a ma.Multiaddr)         {}
func (as *AmbientAutoNAT) ListenClose(net network.Network, a ma.Multiaddr)    {}
func (as *AmbientAutoNAT) OpenedStream(net network.Network, s network.Stream) {}
func (as *AmbientAutoNAT) ClosedStream(net network.Network, s network.Stream) {}

func (as *AmbientAutoNAT) Connected(net network.Network, c network.Conn) {
	if c.Stat().Direction == network.DirInbound &&
		manet.IsPublicAddr(c.RemoteMultiaddr()) {
		select {
		case as.inboundConn <- c:
		default:
		}
	}
}

func (as *AmbientAutoNAT) Disconnected(net network.Network, c network.Conn) {}
