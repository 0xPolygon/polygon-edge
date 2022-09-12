// Deprecated: This package has moved into go-libp2p as a sub-package: github.com/libp2p/go-libp2p/core/routing.
//
// Package routing provides interfaces for peer routing and content routing in libp2p.
package routing

import (
	"context"

	ci "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/routing"
)

// ErrNotFound is returned when the router fails to find the requested record.
// Deprecated: use github.com/libp2p/go-libp2p/core/routing.ErrNotFound instead
var ErrNotFound = routing.ErrNotFound

// ErrNotSupported is returned when the router doesn't support the given record
// type/operation.
// Deprecated: use github.com/libp2p/go-libp2p/core/routing.ErrNotSupported instead
var ErrNotSupported = routing.ErrNotSupported

// ContentRouting is a value provider layer of indirection. It is used to find
// information about who has what content.
//
// Content is identified by CID (content identifier), which encodes a hash
// of the identified content in a future-proof manner.
// Deprecated: use github.com/libp2p/go-libp2p/core/routing.ContentRouting instead
type ContentRouting = routing.ContentRouting

// PeerRouting is a way to find address information about certain peers.
// This can be implemented by a simple lookup table, a tracking server,
// or even a DHT.
// Deprecated: use github.com/libp2p/go-libp2p/core/routing.PeerRouting instead
type PeerRouting = routing.PeerRouting

// ValueStore is a basic Put/Get interface.
// Deprecated: use github.com/libp2p/go-libp2p/core/routing.ValueStore instead
type ValueStore = routing.ValueStore

// Routing is the combination of different routing types supported by libp2p.
// It can be satisfied by a single item (such as a DHT) or multiple different
// pieces that are more optimized to each task.
// Deprecated: use github.com/libp2p/go-libp2p/core/routing.Routing instead
type Routing = routing.Routing

// PubKeyFetcher is an interfaces that should be implemented by value stores
// that can optimize retrieval of public keys.
//
// TODO(steb): Consider removing, see https://github.com/libp2p/go-libp2p-routing/issues/22.
// Deprecated: use github.com/libp2p/go-libp2p/core/routing.PubkeyFetcher instead
type PubKeyFetcher = routing.PubKeyFetcher

// KeyForPublicKey returns the key used to retrieve public keys
// from a value store.
// Deprecated: use github.com/libp2p/go-libp2p/core/routing.KeyForPublicKey instead
func KeyForPublicKey(id peer.ID) string {
	return routing.KeyForPublicKey(id)
}

// GetPublicKey retrieves the public key associated with the given peer ID from
// the value store.
//
// If the ValueStore is also a PubKeyFetcher, this method will call GetPublicKey
// (which may be better optimized) instead of GetValue.
// Deprecated: use github.com/libp2p/go-libp2p/core/routing.GetPublicKey instead
func GetPublicKey(r ValueStore, ctx context.Context, p peer.ID) (ci.PubKey, error) {
	return routing.GetPublicKey(r, ctx, p)
}
