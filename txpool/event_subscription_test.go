package txpool

import (
	"context"
	"crypto/rand"
	"github.com/0xPolygon/polygon-edge/helper/tests"
	"github.com/0xPolygon/polygon-edge/txpool/proto"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
	"math/big"
	mathRand "math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func shuffleTxPoolEvents(
	supportedTypes []proto.EventType,
	count int,
	numInvalid int,
) []*proto.TxPoolEvent {
	if count == 0 || len(supportedTypes) == 0 {
		return []*proto.TxPoolEvent{}
	}

	if numInvalid > count {
		numInvalid = count
	}

	allEvents := []proto.EventType{
		proto.EventType_ADDED,
		proto.EventType_PROMOTED,
		proto.EventType_PROMOTED,
		proto.EventType_DROPPED,
		proto.EventType_DEMOTED,
	}
	txHash := types.StringToHash("123")

	tempSubscription := &eventSubscription{eventTypes: supportedTypes}

	randomEventType := func(supported bool) proto.EventType {
		for {
			randNum, _ := rand.Int(rand.Reader, big.NewInt(int64(len(supportedTypes))))

			randType := allEvents[randNum.Int64()]
			if tempSubscription.eventSupported(randType) == supported {
				return randType
			}
		}
	}

	events := make([]*proto.TxPoolEvent, 0)

	// Fill in the unsupported events first
	for invalidFilled := 0; invalidFilled < numInvalid; invalidFilled++ {
		events = append(events, &proto.TxPoolEvent{
			TxHash: txHash.String(),
			Type:   randomEventType(false),
		})
	}

	// Fill in the supported events
	for validFilled := 0; validFilled < count-numInvalid; validFilled++ {
		events = append(events, &proto.TxPoolEvent{
			TxHash: txHash.String(),
			Type:   randomEventType(true),
		})
	}

	// Shuffle the events
	mathRand.Seed(time.Now().UnixNano())
	mathRand.Shuffle(len(events), func(i, j int) {
		events[i], events[j] = events[j], events[i]
	})

	return events
}

func TestEventSubscription_ProcessedEvents(t *testing.T) {
	// Set up the default values
	supportedEvents := []proto.EventType{
		proto.EventType_ADDED,
		proto.EventType_ENQUEUED,
		proto.EventType_DROPPED,
	}

	testTable := []struct {
		name              string
		events            []*proto.TxPoolEvent
		expectedProcessed int
	}{
		{
			"All supported events processed",
			shuffleTxPoolEvents(supportedEvents, 10, 0),
			10,
		},
		{
			"All unsupported events not processed",
			shuffleTxPoolEvents(supportedEvents, 10, 10),
			0,
		},
		{
			"Mixed events processed",
			shuffleTxPoolEvents(supportedEvents, 10, 6),
			4,
		},
	}

	for _, testCase := range testTable {
		t.Run(testCase.name, func(t *testing.T) {
			subscription := &eventSubscription{
				eventTypes: supportedEvents,
				outputCh:   make(chan *proto.TxPoolEvent, len(testCase.events)),
				doneCh:     make(chan struct{}),
				eventStore: &eventQueue{
					events: make([]*proto.TxPoolEvent, 0),
				},
				notifyCh: make(chan struct{}),
			}
			go subscription.runLoop()

			// Set the event listener
			processed := int64(0)
			go func() {
				for range subscription.outputCh {
					atomic.AddInt64(&processed, 1)
				}
			}()

			// Fire off the events
			var wg sync.WaitGroup
			for _, event := range testCase.events {
				wg.Add(1)
				go func(event *proto.TxPoolEvent) {
					defer wg.Done()

					subscription.pushEvent(event)
				}(event)
			}

			wg.Wait()
			eventWaitCtx, eventWaitFn := context.WithTimeout(context.Background(), time.Second*5)
			defer eventWaitFn()
			if _, err := tests.RetryUntilTimeout(eventWaitCtx, func() (interface{}, bool) {
				return nil, atomic.LoadInt64(&processed) < int64(testCase.expectedProcessed)
			}); err != nil {
				t.Fatalf("Unable to wait for events to be processed, %v", err)
			}

			subscription.close()

			assert.Equal(t, int64(testCase.expectedProcessed), processed)
		})
	}
}

func TestEventSubscription_EventSupported(t *testing.T) {
	t.Parallel()

	supportedEvents := []proto.EventType{
		proto.EventType_ADDED,
		proto.EventType_PROMOTED,
		proto.EventType_DEMOTED,
	}

	subscription := &eventSubscription{
		eventTypes: supportedEvents,
	}

	testTable := []struct {
		name      string
		events    []proto.EventType
		supported bool
	}{
		{
			"Supported events processed",
			supportedEvents,
			true,
		},
		{
			"Unsupported events not processed",
			[]proto.EventType{
				proto.EventType_DROPPED,
				proto.EventType_ENQUEUED,
			},
			false,
		},
	}

	for _, testCase := range testTable {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			for _, eventType := range testCase.events {
				assert.Equal(t, testCase.supported, subscription.eventSupported(eventType))
			}
		})
	}
}
