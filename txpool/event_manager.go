package txpool

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/0xPolygon/polygon-edge/txpool/proto"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/google/uuid"
	"github.com/hashicorp/go-hclog"
)

type eventManager struct {
	subscriptions     map[subscriptionID]*eventSubscription
	subscriptionsLock sync.RWMutex
	numSubscriptions  int64
	logger            hclog.Logger
}

func newEventManager(logger hclog.Logger) *eventManager {
	return &eventManager{
		logger:           logger.Named("event-manager"),
		subscriptions:    make(map[subscriptionID]*eventSubscription),
		numSubscriptions: 0,
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

	id := uuid.New().ID()
	subscription := &eventSubscription{
		eventTypes: eventTypes,
		outputCh:   make(chan *proto.TxPoolEvent),
		doneCh:     make(chan struct{}),
		notifyCh:   make(chan struct{}, 1),
		eventStore: &eventQueue{
			events: make([]*proto.TxPoolEvent, 0),
		},
	}

	em.subscriptions[subscriptionID(id)] = subscription

	go subscription.runLoop()

	em.logger.Info(fmt.Sprintf("Added new subscription %d", id))
	atomic.AddInt64(&em.numSubscriptions, 1)

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
		em.logger.Info(fmt.Sprintf("Canceled subscription %d", id))
		delete(em.subscriptions, id)
		atomic.AddInt64(&em.numSubscriptions, -1)
	}
}

// Close stops the event manager, effectively cancelling all subscriptions
func (em *eventManager) Close() {
	em.subscriptionsLock.Lock()
	defer em.subscriptionsLock.Unlock()

	for _, subscription := range em.subscriptions {
		subscription.close()
	}

	atomic.StoreInt64(&em.numSubscriptions, 0)
}

// signalEvent is a helper method for alerting listeners of a new TxPool event
func (em *eventManager) signalEvent(eventType proto.EventType, txHashes ...types.Hash) {
	if atomic.LoadInt64(&em.numSubscriptions) < 1 {
		// No reason to lock the subscriptions map
		// if no subscriptions exist
		return
	}

	em.subscriptionsLock.RLock()
	defer em.subscriptionsLock.RUnlock()

	for _, txHash := range txHashes {
		for _, subscription := range em.subscriptions {
			subscription.pushEvent(&proto.TxPoolEvent{
				Type:   eventType,
				TxHash: txHash.String(),
			})
		}
	}
}
