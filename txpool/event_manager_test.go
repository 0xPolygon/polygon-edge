package txpool

import (
	"github.com/0xPolygon/polygon-edge/txpool/proto"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
	"sync"
	"testing"
	"time"
)

func TestEventManager_SubscribeCancel(t *testing.T) {
	numSubscriptions := 10
	subscriptions := make([]*subscribeResult, numSubscriptions)
	defaultEvents := []proto.EventType{proto.EventType_PROMOTED}
	IDMap := make(map[subscriptionID]bool)

	em := newEventManager(hclog.NewNullLogger())
	defer em.Close()

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
	em.Close()
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

	defer em.Close()

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

func TestEventManager_SignalEventOrder(t *testing.T) {
	totalEvents := 1000
	supportedEventTypes := []proto.EventType{
		proto.EventType_ADDED,
		proto.EventType_DROPPED,
		proto.EventType_ENQUEUED,
		proto.EventType_DEMOTED,
		proto.EventType_PROMOTED,
	}

	em := newEventManager(hclog.NewNullLogger())

	defer em.Close()

	subscription := em.subscribe(supportedEventTypes)

	mockEvents := shuffleTxPoolEvents(supportedEventTypes, totalEvents, 0)
	mockHash := types.StringToHash(mockEvents[0].TxHash)
	eventsProcessed := 0

	var wg sync.WaitGroup

	wg.Add(totalEvents)

	go func() {
		for {
			select {
			case event, more := <-subscription.subscriptionChannel:
				if more {
					assert.Equal(t, mockEvents[eventsProcessed].Type, event.Type)

					eventsProcessed++

					wg.Done()
				}
			case <-time.After(time.Second * 5):
				for i := 0; i < totalEvents-eventsProcessed; i++ {
					wg.Done()
				}
			}
		}
	}()

	// Send the events
	for _, mockEvent := range mockEvents {
		em.signalEvent(mockEvent.Type, mockHash)
	}

	// Make sure all valid events get processed
	wg.Wait()

	assert.Equal(t, totalEvents, eventsProcessed)
}
