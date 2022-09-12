package event

import (
	"github.com/libp2p/go-libp2p/core/event"
)

// SubscriptionOpt represents a subscriber option. Use the options exposed by the implementation of choice.
// Deprecated: use github.com/libp2p/go-libp2p/core/event.SubscriptionOpt instead
type SubscriptionOpt = event.SubscriptionOpt

// EmitterOpt represents an emitter option. Use the options exposed by the implementation of choice.
// Deprecated: use github.com/libp2p/go-libp2p/core/event.EmitterOpt instead
type EmitterOpt = event.EmitterOpt

// CancelFunc closes a subscriber.
// Deprecated: use github.com/libp2p/go-libp2p/core/event.CancelFunc instead
type CancelFunc = event.CancelFunc

// WildcardSubscription is the type to subscribe to to receive all events
// emitted in the eventbus.
// Deprecated: use github.com/libp2p/go-libp2p/core/event.WildcardSubscription instead
var WildcardSubscription = event.WildcardSubscription

// Emitter represents an actor that emits events onto the eventbus.
// Deprecated: use github.com/libp2p/go-libp2p/core/event.Emitter instead
type Emitter = event.Emitter

// Subscription represents a subscription to one or multiple event types.
// Deprecated: use github.com/libp2p/go-libp2p/core/event.Subscription instead
type Subscription = event.Subscription

// Bus is an interface for a type-based event delivery system.
// Deprecated: use github.com/libp2p/go-libp2p/core/event.Bus instead
type Bus = event.Bus
