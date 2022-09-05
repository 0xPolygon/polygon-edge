// Deprecated: This package has moved into go-libp2p as a sub-package: github.com/libp2p/go-libp2p/core/connmgr.
//
// Package connmgr provides connection tracking and management interfaces for libp2p.
//
// The ConnManager interface exported from this package allows libp2p to enforce an
// upper bound on the total number of open connections. To avoid service disruptions,
// connections can be tagged with metadata and optionally "protected" to ensure that
// essential connections are not arbitrarily cut.
package connmgr

import (
	"github.com/libp2p/go-libp2p/core/connmgr"
)

// SupportsDecay evaluates if the provided ConnManager supports decay, and if
// so, it returns the Decayer object. Refer to godocs on Decayer for more info.
// Deprecated: use github.com/libp2p/go-libp2p/core/connmgr.SupportsDecay instead
func SupportsDecay(mgr ConnManager) (Decayer, bool) {
	return connmgr.SupportsDecay(mgr)
}

// ConnManager tracks connections to peers, and allows consumers to associate
// metadata with each peer.
//
// It enables connections to be trimmed based on implementation-defined
// heuristics. The ConnManager allows libp2p to enforce an upper bound on the
// total number of open connections.
//
// ConnManagers supporting decaying tags implement Decayer. Use the
// SupportsDecay function to safely cast an instance to Decayer, if supported.
// Deprecated: use github.com/libp2p/go-libp2p/core/connmgr.ConnManager instead
type ConnManager = connmgr.ConnManager

// TagInfo stores metadata associated with a peer.
// Deprecated: use github.com/libp2p/go-libp2p/core/connmgr.TagInfo instead
type TagInfo = connmgr.TagInfo
