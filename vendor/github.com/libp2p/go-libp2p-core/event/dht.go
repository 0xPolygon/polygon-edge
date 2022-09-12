package event

import "github.com/libp2p/go-libp2p/core/event"

// RawJSON is a type that contains a raw JSON string.
// Deprecated: use github.com/libp2p/go-libp2p/core/event.RawJSON instead
type RawJSON = event.RawJSON

// GenericDHTEvent is a type that encapsulates an actual DHT event by carrying
// its raw JSON.
//
// Context: the DHT event system is rather bespoke and a bit messy at the time,
// so until we unify/clean that up, this event bridges the gap. It should only
// be consumed for informational purposes.
//
// EXPERIMENTAL: this will likely be removed if/when the DHT event types are
// hoisted to core, and the DHT event system is reconciled with the eventbus.
// Deprecated: use github.com/libp2p/go-libp2p/core/event.GenericDHTEvent instead
type GenericDHTEvent = event.GenericDHTEvent
