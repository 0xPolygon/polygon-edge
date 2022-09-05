package network

import (
	"github.com/libp2p/go-libp2p/core/network"
)

// Conn is a connection to a remote peer. It multiplexes streams.
// Usually there is no need to use a Conn directly, but it may
// be useful to get information about the peer on the other side:
//
//	stream.Conn().RemotePeer()
//
// Deprecated: use github.com/libp2p/go-libp2p/core/network.Conn instead
type Conn = network.Conn

// ConnSecurity is the interface that one can mix into a connection interface to
// give it the security methods.
// Deprecated: use github.com/libp2p/go-libp2p/core/network.ConnSecurity instead
type ConnSecurity = network.ConnSecurity

// ConnMultiaddrs is an interface mixin for connection types that provide multiaddr
// addresses for the endpoints.
// Deprecated: use github.com/libp2p/go-libp2p/core/network.ConnMultiaddrs instead
type ConnMultiaddrs = network.ConnMultiaddrs

// ConnStat is an interface mixin for connection types that provide connection statistics.
// Deprecated: use github.com/libp2p/go-libp2p/core/network.ConnStat instead
type ConnStat = network.ConnStats

// ConnScoper is the interface that one can mix into a connection interface to give it a resource
// management scope
// Deprecated: use github.com/libp2p/go-libp2p/core/network.ConnScoper instead
type ConnScoper = network.ConnScoper
