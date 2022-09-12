package event

import (
	"github.com/libp2p/go-libp2p/core/event"
)

// EvtPeerProtocolsUpdated should be emitted when a peer we're connected to adds or removes protocols from their stack.
// Deprecated: use github.com/libp2p/go-libp2p/core/event.EvtPeerProtocolsUpdated instead
type EvtPeerProtocolsUpdated = event.EvtPeerProtocolsUpdated

// EvtLocalProtocolsUpdated should be emitted when stream handlers are attached or detached from the local host.
// For handlers attached with a matcher predicate (host.SetStreamHandlerMatch()), only the protocol ID will be
// included in this event.
// Deprecated: use github.com/libp2p/go-libp2p/core/event.EvtLocalProtocolsUpdated instead
type EvtLocalProtocolsUpdated = event.EvtLocalProtocolsUpdated
