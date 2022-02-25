// Package websocket implements a websocket based transport for go-libp2p.
package websocket

import (
	"context"

	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/transport"

	ma "github.com/multiformats/go-multiaddr"
	mafmt "github.com/multiformats/go-multiaddr-fmt"
	manet "github.com/multiformats/go-multiaddr/net"
)

// WsFmt is multiaddr formatter for WsProtocol
var WsFmt = mafmt.And(mafmt.TCP, mafmt.Base(ma.P_WS))

// This is _not_ WsFmt because we want the transport to stick to dialing fully
// resolved addresses.
var dialMatcher = mafmt.And(mafmt.IP, mafmt.Base(ma.P_TCP), mafmt.Base(ma.P_WS))

func init() {
	manet.RegisterFromNetAddr(ParseWebsocketNetAddr, "websocket")
	manet.RegisterToNetAddr(ConvertWebsocketMultiaddrToNetAddr, "ws")
}

var _ transport.Transport = (*WebsocketTransport)(nil)

// WebsocketTransport is the actual go-libp2p transport
type WebsocketTransport struct {
	upgrader transport.Upgrader
	rcmgr    network.ResourceManager
}

func New(u transport.Upgrader, rcmgr network.ResourceManager) *WebsocketTransport {
	if rcmgr == nil {
		rcmgr = network.NullResourceManager
	}
	return &WebsocketTransport{
		upgrader: u,
		rcmgr:    rcmgr,
	}
}

func (t *WebsocketTransport) CanDial(a ma.Multiaddr) bool {
	return dialMatcher.Matches(a)
}

func (t *WebsocketTransport) Protocols() []int {
	return []int{ma.ProtocolWithCode(ma.P_WS).Code}
}

func (t *WebsocketTransport) Proxy() bool {
	return false
}

func (t *WebsocketTransport) Dial(ctx context.Context, raddr ma.Multiaddr, p peer.ID) (transport.CapableConn, error) {
	connScope, err := t.rcmgr.OpenConnection(network.DirOutbound, true)
	if err != nil {
		return nil, err
	}
	macon, err := t.maDial(ctx, raddr)
	if err != nil {
		connScope.Done()
		return nil, err
	}
	return t.upgrader.Upgrade(ctx, t, macon, network.DirOutbound, p, connScope)
}
