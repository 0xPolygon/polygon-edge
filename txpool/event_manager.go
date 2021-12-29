package txpool

import (
	"fmt"
	"github.com/0xPolygon/polygon-sdk/txpool/proto"
	"github.com/google/uuid"
	"github.com/hashicorp/go-hclog"
	"sync"
)

type eventManager struct {
	subscriptions     map[subscriptionID]*eventSubscription
	subscriptionsLock sync.RWMutex
	logger            hclog.Logger
}

func newEventManager(logger hclog.Logger) *eventManager {
	return &eventManager{
		logger:        logger.Named("event-manager"),
		subscriptions: make(map[subscriptionID]*eventSubscription),
	}
}

type subscribeResult struct {
	subscriptionID      subscriptionID
	subscriptionChannel chan *proto.TxPoolEvent
}

// subscribe registers a new listener for TxPool events
func (em *eventManager) subscribe(eventTypes []proto.EventType) *subscribeResult {
	em.subscriptionsLock.Lock()
	defer em.subscriptionsLock.Unlock()

	id := uuid.New().String()
	subscription := &eventSubscription{
		eventTypes: eventTypes,
		processCh:  make(chan *proto.TxPoolEvent),
		outputCh:   make(chan *proto.TxPoolEvent),
	}

	em.subscriptions[subscriptionID(id)] = subscription
	em.logger.Info(fmt.Sprintf("Added new subscription %s", id))
	go subscription.runLoop()

	return &subscribeResult{
		subscriptionID:      subscriptionID(id),
		subscriptionChannel: subscription.outputCh,
	}
}

// cancelSubscription stops a subscription for TxPool events
func (em *eventManager) cancelSubscription(id subscriptionID) {
	em.subscriptionsLock.Lock()
	defer em.subscriptionsLock.Unlock()

	if subscription, ok := em.subscriptions[id]; ok {
		subscription.close()
		em.logger.Info(fmt.Sprintf("Canceled subscription %s", id))
	}
}

// close stops the event manager, effectively cancelling all subscriptions
func (em *eventManager) close() {
	em.subscriptionsLock.Lock()
	defer em.subscriptionsLock.Unlock()

	for _, subscription := range em.subscriptions {
		subscription.close()
	}
}

// fireEvent is a helper method for alerting listeners of a new TxPool event
func (em *eventManager) fireEvent(event *proto.TxPoolEvent) {
	em.subscriptionsLock.RLock()
	defer em.subscriptionsLock.RUnlock()

	for _, subscription := range em.subscriptions {
		go func(eventSubscription *eventSubscription) {
			eventSubscription.processCh <- event
		}(subscription)
	}
}
