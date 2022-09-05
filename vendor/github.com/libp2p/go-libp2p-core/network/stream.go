package network

import (
	"github.com/libp2p/go-libp2p/core/network"
)

// Stream represents a bidirectional channel between two agents in
// a libp2p network. "agent" is as granular as desired, potentially
// being a "request -> reply" pair, or whole protocols.
//
// Streams are backed by a multiplexer underneath the hood.
// Deprecated: use github.com/libp2p/go-libp2p/core/network.Stream instead
type Stream = network.Stream
