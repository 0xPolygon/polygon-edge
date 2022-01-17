package txpool

import (
	"github.com/0xPolygon/polygon-sdk/txpool/proto"
	"github.com/0xPolygon/polygon-sdk/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestEventManager_SubscribeCancel(t *testing.T) {
	numSubscriptions := 10
	subscriptions := make([]*subscribeResult, numSubscriptions)
	defaultEvents := []proto.EventType{proto.EventType_PROMOTED}
	IDMap := make(map[subscriptionID]bool)

	em := newEventManager(hclog.NewNullLogger())
	defer em.close()

	// Create the subscriptions
	for i := 0; i < numSubscriptions; i++ {
		subscriptions[i] = em.subscribe(defaultEvents)

		// Check that the number is up-to-date
		assert.Equal(t, int64(i+1), em.numSubscriptions)

		// Check if a duplicate ID has been issued
		if _, ok := IDMap[subscriptions[i].subscriptionID]; ok {
			t.Fatalf("Duplicate ID entry")
		} else {
			IDMap[subscriptions[i].subscriptionID] = true
		}
	}

	// Cancel them one by one
	for indx, subscription := range subscriptions {
		em.cancelSubscription(subscription.subscriptionID)

		// Check that the number is up-to-date
		assert.Equal(t, int64(numSubscriptions-indx-1), em.numSubscriptions)

		// Check that the appropriate channel is closed
		if _, more := <-subscription.subscriptionChannel; more {
			t.Fatalf("Subscription channel not closed for index %d", indx)
		}
	}
}

func TestEventManager_SubscribeClose(t *testing.T) {
	numSubscriptions := 10
	subscriptions := make([]*subscribeResult, numSubscriptions)
	defaultEvents := []proto.EventType{proto.EventType_PROMOTED}

	em := newEventManager(hclog.NewNullLogger())

	// Create the subscriptions
	for i := 0; i < numSubscriptions; i++ {
		subscriptions[i] = em.subscribe(defaultEvents)

		// Check that the number is up-to-date
		assert.Equal(t, int64(i+1), em.numSubscriptions)
	}

	// Close off the event manager
	em.close()
	assert.Equal(t, int64(0), em.numSubscriptions)

	// Check if the subscription channels are closed
	for indx, subscription := range subscriptions {
		if _, more := <-subscription.subscriptionChannel; more {
			t.Fatalf("Subscription channel not closed for index %d", indx)
		}
	}
}

func TestEventManager_SignalEvent(t *testing.T) {
	totalEvents := 10
	invalidEvents := 3
	validEvents := totalEvents - invalidEvents
	supportedEventTypes := []proto.EventType{proto.EventType_ADDED, proto.EventType_DROPPED}

	em := newEventManager(hclog.NewNullLogger())
	defer em.close()

	subscription := em.subscribe(supportedEventTypes)

	eventSupported := func(eventType proto.EventType) bool {
		for _, supportedType := range supportedEventTypes {
			if supportedType == eventType {
				return true
			}
		}

		return false
	}

	mockEvents := shuffleTxPoolEvents(supportedEventTypes, totalEvents, invalidEvents)
	mockHash := types.StringToHash(mockEvents[0].TxHash)

	// Send the events
	for _, mockEvent := range mockEvents {
		em.signalEvent(mockEvent.Type, mockHash)
	}

	// Make sure all valid events get processed
	eventsProcessed := 0
	supportedEventsProcessed := 0

	completed := false
	for !completed {
		select {
		case event := <-subscription.subscriptionChannel:
			eventsProcessed++

			if eventSupported(event.Type) {
				supportedEventsProcessed++
			}

			if eventsProcessed == validEvents ||
				supportedEventsProcessed == validEvents {
				completed = true
			}
		case <-time.After(time.Second * 5):
			completed = true
		}
	}

	assert.Equal(t, validEvents, eventsProcessed)
	assert.Equal(t, validEvents, supportedEventsProcessed)
}
