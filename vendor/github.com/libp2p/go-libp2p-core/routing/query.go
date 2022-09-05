package routing

import (
	"context"

	"github.com/libp2p/go-libp2p/core/routing"
)

// QueryEventType indicates the query event's type.
// Deprecated: use github.com/libp2p/go-libp2p/core/routing.QueryEventType instead
type QueryEventType = routing.QueryEventType

// Number of events to buffer.
// Deprecated: use github.com/libp2p/go-libp2p/core/routing.QueryEventBufferSize instead
var QueryEventBufferSize = routing.QueryEventBufferSize

const (
	// Sending a query to a peer.
	// Deprecated: use github.com/libp2p/go-libp2p/core/routing.SendingQuery instead
	SendingQuery = routing.SendingQuery
	// Got a response from a peer.
	// Deprecated: use github.com/libp2p/go-libp2p/core/routing.PeerResponse instead
	PeerResponse = routing.PeerResponse
	// Found a "closest" peer (not currently used).
	// Deprecated: use github.com/libp2p/go-libp2p/core/routing.FinalPeer instead
	FinalPeer = routing.FinalPeer
	// Got an error when querying.
	// Deprecated: use github.com/libp2p/go-libp2p/core/routing.QueryError instead
	QueryError = routing.QueryError
	// Found a provider.
	// Deprecated: use github.com/libp2p/go-libp2p/core/routing.Provider instead
	Provider = routing.Provider
	// Found a value.
	// Deprecated: use github.com/libp2p/go-libp2p/core/routing.Value instead
	Value = routing.Value
	// Adding a peer to the query.
	// Deprecated: use github.com/libp2p/go-libp2p/core/routing.AddingPeer instead
	AddingPeer = routing.AddingPeer
	// Dialing a peer.
	// Deprecated: use github.com/libp2p/go-libp2p/core/routing.DialingPeer instead
	DialingPeer = routing.DialingPeer
)

// QueryEvent is emitted for every notable event that happens during a DHT query.
// Deprecated: use github.com/libp2p/go-libp2p/core/routing.QueryEvent instead
type QueryEvent = routing.QueryEvent

// RegisterForQueryEvents registers a query event channel with the given
// context. The returned context can be passed to DHT queries to receive query
// events on the returned channels.
//
// The passed context MUST be canceled when the caller is no longer interested
// in query events.
// Deprecated: use github.com/libp2p/go-libp2p/core/routing.RegisterForQueryEvents instead
func RegisterForQueryEvents(ctx context.Context) (context.Context, <-chan *QueryEvent) {
	return routing.RegisterForQueryEvents(ctx)
}

// PublishQueryEvent publishes a query event to the query event channel
// associated with the given context, if any.
// Deprecated: use github.com/libp2p/go-libp2p/core/routing.PublishQueryEvent instead
func PublishQueryEvent(ctx context.Context, ev *QueryEvent) {
	routing.PublishQueryEvent(ctx, ev)
}

// SubscribesToQueryEvents returns true if the context subscribes to query
// events. If this function returns falls, calling `PublishQueryEvent` on the
// context will be a no-op.
// Deprecated: use github.com/libp2p/go-libp2p/core/routing.SubscribesToQueryEvents instead
func SubscribesToQueryEvents(ctx context.Context) bool {
	return routing.SubscribesToQueryEvents(ctx)
}
