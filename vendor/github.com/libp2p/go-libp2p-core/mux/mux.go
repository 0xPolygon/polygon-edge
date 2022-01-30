// Package mux provides stream multiplexing interfaces for libp2p.
//
// For a conceptual overview of stream multiplexing in libp2p, see
// https://docs.libp2p.io/concepts/stream-multiplexing/
package mux

import (
	"github.com/libp2p/go-libp2p-core/network"
)

// Deprecated: use network.ErrReset instead.
var ErrReset = network.ErrReset

// Deprecated: use network.MuxedStream instead.
type MuxedStream = network.MuxedStream

// Deprecated: use network.MuxedConn instead.
type MuxedConn = network.MuxedConn

// Deprecated: use network.Multiplexer instead.
type Multiplexer = network.Multiplexer
