package routing

import "github.com/libp2p/go-libp2p/core/routing"

// Option is a single routing option.
// Deprecated: use github.com/libp2p/go-libp2p/core/routing.Option instead
type Option = routing.Option

// Options is a set of routing options
// Deprecated: use github.com/libp2p/go-libp2p/core/routing.Options instead
type Options = routing.Options

// Expired is an option that tells the routing system to return expired records
// when no newer records are known.
// Deprecated: use github.com/libp2p/go-libp2p/core/routing.Expired instead
var Expired = routing.Expired

// Offline is an option that tells the routing system to operate offline (i.e., rely on cached/local data only).
// Deprecated: use github.com/libp2p/go-libp2p/core/routing.Offline instead
var Offline = routing.Offline
