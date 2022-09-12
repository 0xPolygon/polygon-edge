package network

import (
	"github.com/libp2p/go-libp2p/core/network"
)

// ErrReset is returned when reading or writing on a reset stream.
// Deprecated: use github.com/libp2p/go-libp2p/core/network.ErrReset instead
var ErrReset = network.ErrReset

// MuxedStream is a bidirectional io pipe within a connection.
// Deprecated: use github.com/libp2p/go-libp2p/core/network.MuxedStream instead
type MuxedStream = network.MuxedStream

// MuxedConn represents a connection to a remote peer that has been
// extended to support stream multiplexing.
//
// A MuxedConn allows a single net.Conn connection to carry many logically
// independent bidirectional streams of binary data.
//
// Together with network.ConnSecurity, MuxedConn is a component of the
// transport.CapableConn interface, which represents a "raw" network
// connection that has been "upgraded" to support the libp2p capabilities
// of secure communication and stream multiplexing.
// Deprecated: use github.com/libp2p/go-libp2p/core/network.MuxedConn instead
type MuxedConn = network.MuxedConn

// Multiplexer wraps a net.Conn with a stream multiplexing
// implementation and returns a MuxedConn that supports opening
// multiple streams over the underlying net.Conn
// Deprecated: use github.com/libp2p/go-libp2p/core/network.Multiplexer instead
type Multiplexer = network.Multiplexer
