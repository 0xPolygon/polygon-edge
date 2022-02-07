package peerstream_multiplex

import (
	"net"

	"github.com/libp2p/go-libp2p-core/network"

	mp "github.com/libp2p/go-mplex"
)

// DefaultTransport has default settings for Transport
var DefaultTransport = &Transport{}

var _ network.Multiplexer = &Transport{}

// Transport implements mux.Multiplexer that constructs
// mplex-backed muxed connections.
type Transport struct{}

func (t *Transport) NewConn(nc net.Conn, isServer bool, scope network.PeerScope) (network.MuxedConn, error) {
	return (*conn)(mp.NewMultiplex(nc, isServer, scope)), nil
}
