// Package protocol provides core interfaces for protocol routing and negotiation in libp2p.
package protocol

import (
	"github.com/libp2p/go-libp2p/core/protocol"
)

// HandlerFunc is a user-provided function used by the Router to
// handle a protocol/stream.
//
// Will be invoked with the protocol ID string as the first argument,
// which may differ from the ID used for registration if the handler
// was registered using a match function.
// Deprecated: use github.com/libp2p/go-libp2p/core/protocol.HandlerFunc instead
type HandlerFunc = protocol.HandlerFunc

// Router is an interface that allows users to add and remove protocol handlers,
// which will be invoked when incoming stream requests for registered protocols
// are accepted.
//
// Upon receiving an incoming stream request, the Router will check all registered
// protocol handlers to determine which (if any) is capable of handling the stream.
// The handlers are checked in order of registration; if multiple handlers are
// eligible, only the first to be registered will be invoked.
// Deprecated: use github.com/libp2p/go-libp2p/core/protocol.Router instead
type Router = protocol.Router

// Negotiator is a component capable of reaching agreement over what protocols
// to use for inbound streams of communication.
// Deprecated: use github.com/libp2p/go-libp2p/core/protocol.Negotiator instead
type Negotiator = protocol.Negotiator

// Switch is the component responsible for "dispatching" incoming stream requests to
// their corresponding stream handlers. It is both a Negotiator and a Router.
// Deprecated: use github.com/libp2p/go-libp2p/core/protocol.Switch instead
type Switch = protocol.Switch
