package network

import (
	"github.com/libp2p/go-libp2p/core/network"
)

// Notifiee is an interface for an object wishing to receive
// notifications from a Network.
// Deprecated: use github.com/libp2p/go-libp2p/core/network.Notifiee instead
type Notifiee = network.Notifiee

// NotifyBundle implements Notifiee by calling any of the functions set on it,
// and nop'ing if they are unset. This is the easy way to register for
// notifications.
// Deprecated: use github.com/libp2p/go-libp2p/core/network.NotifyBundle instead
type NotifyBundle = network.NotifyBundle

// Global noop notifiee. Do not change.
// Deprecated: use github.com/libp2p/go-libp2p/core/network.GlobalNoopNotifiee instead
var GlobalNoopNotifiee = network.GlobalNoopNotifiee

// Deprecated: use github.com/libp2p/go-libp2p/core/network.NoopNotifiee instead
type NoopNotifiee = network.NoopNotifiee
