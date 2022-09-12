// Deprecated: This package has moved into go-libp2p as a sub-package: github.com/libp2p/go-libp2p/core/host.
//
// Package host provides the core Host interface for libp2p.
//
// Host represents a single libp2p node in a peer-to-peer network.
package host

import (
	"github.com/libp2p/go-libp2p/core/host"
)

// Host is an object participating in a p2p network, which
// implements protocols or provides services. It handles
// requests like a Server, and issues requests like a Client.
// It is called Host because it is both Server and Client (and Peer
// may be confusing).
// Deprecated: use github.com/libp2p/go-libp2p/core/host.Host instead
type Host = host.Host

// IntrospectableHost is implemented by Host implementations that are
// introspectable, that is, that may have introspection capability.
// Deprecated: use github.com/libp2p/go-libp2p/core/host.IntrospectableHost instead
type IntrospectableHost = host.IntrospectableHost
