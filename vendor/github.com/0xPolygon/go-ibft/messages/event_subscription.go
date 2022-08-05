package messages

import (
	"github.com/0xPolygon/go-ibft/messages/proto"
)

type eventSubscription struct {
	// details contains the details of the event subscription
	details SubscriptionDetails

	// outputCh is the update channel for the subscriber
	outputCh chan uint64

	// doneCh is the channel for handling stop signals
	doneCh chan struct{}

	// notifyCh is the channel for receiving event requests
	notifyCh chan uint64
}

// close stops the event subscription
func (es *eventSubscription) close() {
	close(es.doneCh)
}

// runLoop is the main loop that listens for notifications and handles the event / close signals
func (es *eventSubscription) runLoop() {
	defer close(es.outputCh)

	for {
		select {
		case <-es.doneCh: // Break if a close signal has been received
			return
		case round := <-es.notifyCh: // Listen for new events to appear
			select {
			case <-es.doneCh: // Break if a close signal has been received
				return
			case es.outputCh <- round: // Pass the event to the output
			}
		}
	}
}

// eventSupported checks if any notification event needs to be triggered
func (es *eventSubscription) eventSupported(
	messageType proto.MessageType,
	view *proto.View,
	totalMessages int,
) bool {
	// The heights must match
	if view.Height != es.details.View.Height {
		return false
	}

	// Check the round constraints
	if es.details.HasMinRound {
		// The round can be treated as a min round (message round can be equal or higher)
		if view.Round < es.details.View.Round {
			return false
		}
	} else {
		// The rounds must match
		if view.Round != es.details.View.Round {
			return false
		}
	}

	// The type of message must match
	if messageType != es.details.MessageType {
		return false
	}

	// The total number of messages must be
	// greater of equal to the subscription threshold
	return totalMessages >= es.details.MinNumMessages
}

// pushEvent sends the event off for processing by the subscription. [NON-BLOCKING]
func (es *eventSubscription) pushEvent(
	messageType proto.MessageType,
	view *proto.View,
	totalMessages int,
) {
	if !es.eventSupported(messageType, view, totalMessages) {
		return
	}

	select {
	case es.notifyCh <- view.Round: // Notify the worker thread
	default:
	}
}
