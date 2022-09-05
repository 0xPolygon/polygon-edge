package event

import (
	"github.com/libp2p/go-libp2p/core/event"
)

// EvtPeerIdentificationCompleted is emitted when the initial identification round for a peer is completed.
// Deprecated: use github.com/libp2p/go-libp2p/core/event.EvtPeerIdentificationCompleted instead
type EvtPeerIdentificationCompleted = event.EvtPeerIdentificationCompleted

// EvtPeerIdentificationFailed is emitted when the initial identification round for a peer failed.
// Deprecated: use github.com/libp2p/go-libp2p/core/event.EvtPeerIdentificationFailed instead
type EvtPeerIdentificationFailed = event.EvtPeerIdentificationFailed
