package txpool

import (
	"github.com/0xPolygon/polygon-sdk/txpool/proto"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
	"testing"
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
		// Check that the appropriate channel is closed
		if _, more := <-subscription.subscriptionChannel; more {
			t.Fatalf("Subscription channel not closed for index %d", indx)
		}
	}
}
