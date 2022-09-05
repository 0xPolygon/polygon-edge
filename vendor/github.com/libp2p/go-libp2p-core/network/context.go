package network

import (
	"context"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
)

// DialPeerTimeout is the default timeout for a single call to `DialPeer`. When
// there are multiple concurrent calls to `DialPeer`, this timeout will apply to
// each independently.
// Deprecated: use github.com/libp2p/go-libp2p/core/network.DialPeerTimeout instead
var DialPeerTimeout = network.DialPeerTimeout

// EXPERIMENTAL
// WithForceDirectDial constructs a new context with an option that instructs the network
// to attempt to force a direct connection to a peer via a dial even if a proxied connection to it already exists.
// Deprecated: use github.com/libp2p/go-libp2p/core/network.WithForceDirectDial instead
func WithForceDirectDial(ctx context.Context, reason string) context.Context {
	return network.WithForceDirectDial(ctx, reason)
}

// EXPERIMENTAL
// GetForceDirectDial returns true if the force direct dial option is set in the context.
// Deprecated: use github.com/libp2p/go-libp2p/core/network.GetForceDirectDial instead
func GetForceDirectDial(ctx context.Context) (forceDirect bool, reason string) {
	return network.GetForceDirectDial(ctx)
}

// WithSimultaneousConnect constructs a new context with an option that instructs the transport
// to apply hole punching logic where applicable.
// EXPERIMENTAL
// Deprecated: use github.com/libp2p/go-libp2p/core/network.WithSimultaneousConnect instead
func WithSimultaneousConnect(ctx context.Context, isClient bool, reason string) context.Context {
	return network.WithSimultaneousConnect(ctx, isClient, reason)
}

// GetSimultaneousConnect returns true if the simultaneous connect option is set in the context.
// EXPERIMENTAL
// Deprecated: use github.com/libp2p/go-libp2p/core/network.GetSimultaneousConnect instead
func GetSimultaneousConnect(ctx context.Context) (simconnect bool, isClient bool, reason string) {
	return network.GetSimultaneousConnect(ctx)
}

// WithNoDial constructs a new context with an option that instructs the network
// to not attempt a new dial when opening a stream.
// Deprecated: use github.com/libp2p/go-libp2p/core/network.WithNoDial instead
func WithNoDial(ctx context.Context, reason string) context.Context {
	return network.WithNoDial(ctx, reason)
}

// GetNoDial returns true if the no dial option is set in the context.
// Deprecated: use github.com/libp2p/go-libp2p/core/network.GetNoDial instead
func GetNoDial(ctx context.Context) (nodial bool, reason string) {
	return network.GetNoDial(ctx)
}

// GetDialPeerTimeout returns the current DialPeer timeout (or the default).
// Deprecated: use github.com/libp2p/go-libp2p/core/network.GetDialPeerTimeout instead
func GetDialPeerTimeout(ctx context.Context) time.Duration {
	return network.GetDialPeerTimeout(ctx)
}

// WithDialPeerTimeout returns a new context with the DialPeer timeout applied.
//
// This timeout overrides the default DialPeerTimeout and applies per-dial
// independently.
// Deprecated: use github.com/libp2p/go-libp2p/core/network.WithDialPeerTimeout instead
func WithDialPeerTimeout(ctx context.Context, timeout time.Duration) context.Context {
	return network.WithDialPeerTimeout(ctx, timeout)
}

// WithUseTransient constructs a new context with an option that instructs the network
// that it is acceptable to use a transient connection when opening a new stream.
// Deprecated: use github.com/libp2p/go-libp2p/core/network.WithUseTransient instead
func WithUseTransient(ctx context.Context, reason string) context.Context {
	return network.WithUseTransient(ctx, reason)
}

// GetUseTransient returns true if the use transient option is set in the context.
// Deprecated: use github.com/libp2p/go-libp2p/core/network.GetUseTransient instead
func GetUseTransient(ctx context.Context) (usetransient bool, reason string) {
	return network.GetUseTransient(ctx)
}
