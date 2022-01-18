package txpool

import (
	"github.com/0xPolygon/polygon-sdk/txpool/proto"
)

type subscriptionID int32

type eventSubscription struct {
	// eventTypes is the list of subscribed event types
	eventTypes []proto.EventType

	// outputCh is the update channel for the subscriber
	outputCh chan *proto.TxPoolEvent

	// doneCh indicating that the subscription is terminated
	doneCh chan struct{}

	notifyCh chan struct{}

	eventStore *eventQueue
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
	close(es.doneCh)
	close(es.outputCh)
}

// runLoop is the main loop that listens for notifications and handles the event / close signals
func (es *eventSubscription) runLoop() {
	for {
		select {
		case <-es.doneCh:
			return
		case <-es.notifyCh:
			event := es.eventStore.pop()
			if event == nil {
				continue
			}

			select {
			case es.outputCh <- event: // Pass the event to the output
			case <-es.doneCh: // Break if a close signal has been received
				return
			}
		}
	}
}

// pushEvent sends the event off for processing by the subscription. [NON-BLOCKING]
func (es *eventSubscription) pushEvent(event *proto.TxPoolEvent) {
	if es.eventSupported(event.Type) {
		es.eventStore.push(event)

		go func() {
			select {
			case es.notifyCh <- struct{}{}: // Notify the worker thread
			case <-es.doneCh: // Break if a close signal has been received
				return
			}
		}()
	}
}
