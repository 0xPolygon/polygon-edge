package txpool

import (
	"github.com/0xPolygon/polygon-sdk/txpool/proto"
)

type subscriptionID string

type eventSubscription struct {
	// eventTypes is the list of subscribed event types
	eventTypes []proto.EventType

	// processCh is the intermediary channel between the eventManager and the event subscriber
	processCh chan *proto.TxPoolEvent

	// outputCh is the update channel for the subscriber
	outputCh chan *proto.TxPoolEvent

	pendingMessages uint64
}

// eventSupported checks if the event is supported by the subscription
func (es *eventSubscription) eventSupported(eventType proto.EventType) bool {
	for _, supportedType := range es.eventTypes {
		if supportedType == eventType {
			return true
		}
	}

	return false
}

// close stops the event subscription
func (es *eventSubscription) close() {
	close(es.processCh)
}

// runLoop is the main subscription loop that does event processing
// and listener notification
func (es *eventSubscription) runLoop() {
	for event := range es.processCh {
		if es.eventSupported(event.Type) {
			es.outputCh <- event
		}
	}

	close(es.outputCh)
}
