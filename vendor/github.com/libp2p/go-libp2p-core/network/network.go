// Deprecated: This package has moved into go-libp2p as a sub-package: github.com/libp2p/go-libp2p/core/network.
//
// Package network provides core networking abstractions for libp2p.
//
// The network package provides the high-level Network interface for interacting
// with other libp2p peers, which is the primary public API for initiating and
// accepting connections to remote peers.
package network

import (
	"github.com/libp2p/go-libp2p/core/network"
)

// MessageSizeMax is a soft (recommended) maximum for network messages.
// One can write more, as the interface is a stream. But it is useful
// to bunch it up into multiple read/writes when the whole message is
// a single, large serialized object.
// Deprecated: use github.com/libp2p/go-libp2p/core/network.MessageSizeMax instead
const MessageSizeMax = network.MessageSizeMax

// Direction represents which peer in a stream initiated a connection.
// Deprecated: use github.com/libp2p/go-libp2p/core/network.Direction instead
type Direction = network.Direction

const (
	// DirUnknown is the default direction.
	// Deprecated: use github.com/libp2p/go-libp2p/core/network.DirUnknown instead
	DirUnknown = network.DirUnknown
	// DirInbound is for when the remote peer initiated a connection.
	// Deprecated: use github.com/libp2p/go-libp2p/core/network.DirInbound instead
	DirInbound = network.DirInbound
	// DirOutbound is for when the local peer initiated a connection.
	// Deprecated: use github.com/libp2p/go-libp2p/core/network.DirOutbound instead
	DirOutbound = network.DirOutbound
)

// Connectedness signals the capacity for a connection with a given node.
// It is used to signal to services and other peers whether a node is reachable.
type Connectedness = network.Connectedness

const (
	// NotConnected means no connection to peer, and no extra information (default)
	// Deprecated: use github.com/libp2p/go-libp2p/core/network.NotConnected instead
	NotConnected = network.NotConnected

	// Connected means has an open, live connection to peer
	// Deprecated: use github.com/libp2p/go-libp2p/core/network.Connected instead
	Connected = network.Connected

	// CanConnect means recently connected to peer, terminated gracefully
	// Deprecated: use github.com/libp2p/go-libp2p/core/network.CanConnect instead
	CanConnect = network.CanConnect

	// CannotConnect means recently attempted connecting but failed to connect.
	// (should signal "made effort, failed")
	// Deprecated: use github.com/libp2p/go-libp2p/core/network.CannotConnect instead
	CannotConnect = network.CannotConnect
)

// Reachability indicates how reachable a node is.
// Deprecated: use github.com/libp2p/go-libp2p/core/network.Reachability instead
type Reachability = network.Reachability

const (
	// ReachabilityUnknown indicates that the reachability status of the
	// node is unknown.
	// Deprecated: use github.com/libp2p/go-libp2p/core/network.ReachabilityUnknown instead
	ReachabilityUnknown = network.ReachabilityUnknown

	// ReachabilityPublic indicates that the node is reachable from the
	// public internet.
	// Deprecated: use github.com/libp2p/go-libp2p/core/network.ReachabilityPubic instead
	ReachabilityPublic = network.ReachabilityPublic

	// ReachabilityPrivate indicates that the node is not reachable from the
	// public internet.
	//
	// NOTE: This node may _still_ be reachable via relays.
	// Deprecated: use github.com/libp2p/go-libp2p/core/network.ReachabilityPrivate instead
	ReachabilityPrivate = network.ReachabilityPrivate
)

// ConnStats stores metadata pertaining to a given Conn.
// Deprecated: use github.com/libp2p/go-libp2p/core/network.ConnStats instead
type ConnStats = network.ConnStats

// Stats stores metadata pertaining to a given Stream / Conn.
// Deprecated: use github.com/libp2p/go-libp2p/core/network.Stats instead
type Stats = network.Stats

// StreamHandler is the type of function used to listen for
// streams opened by the remote side.
// Deprecated: use github.com/libp2p/go-libp2p/core/network.StreamHandler instead
type StreamHandler = network.StreamHandler

// Network is the interface used to connect to the outside world.
// It dials and listens for connections. it uses a Swarm to pool
// connections (see swarm pkg, and peerstream.Swarm). Connections
// are encrypted with a TLS-like protocol.
// Deprecated: use github.com/libp2p/go-libp2p/core/network.Network instead
type Network = network.Network

// Dialer represents a service that can dial out to peers
// (this is usually just a Network, but other services may not need the whole
// stack, and thus it becomes easier to mock)
// Deprecated: use github.com/libp2p/go-libp2p/core/network.Dialer instead
type Dialer = network.Dialer
