package txpool

import (
	"github.com/0xPolygon/polygon-sdk/txpool/proto"
	"github.com/0xPolygon/polygon-sdk/types"
	"github.com/stretchr/testify/assert"
	"math/rand"
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
			randType := allEvents[rand.Intn(len(supportedTypes))]
			if tempSubscription.eventSupported(randType) == supported {
				return randType
			}
		}
	}

	rand.Seed(time.Now().UnixNano())
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
	rand.Shuffle(len(events), func(i, j int) {
		events[i], events[j] = events[j], events[i]
	})

	return events
}

func TestEventSubscription_RunLoop(t *testing.T) {
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
			10 - 0,
		},
		{
			"All unsupported events not processed",
			shuffleTxPoolEvents(supportedEvents, 10, 10),
			10 - 10,
		},
		{
			"Mixed events processed",
			shuffleTxPoolEvents(supportedEvents, 10, 6),
			10 - 6,
		},
	}

	for _, testCase := range testTable {
		t.Run(testCase.name, func(t *testing.T) {
			subscription := &eventSubscription{
				eventTypes: supportedEvents,
				outputCh:   make(chan *proto.TxPoolEvent),
				doneCh:     make(chan struct{}),
			}

			// Fire off the events
			for _, event := range testCase.events {
				subscription.pushEvent(event)
			}

			// Wait for the listener to process them
			processed := 0
			completed := false

			for !completed {
				select {
				case <-time.After(time.Second * 5):
					completed = true
				case <-subscription.outputCh:
					processed++
					if processed == testCase.expectedProcessed {
						completed = true
					}
				}
			}

			assert.Equal(t, testCase.expectedProcessed, processed)
			subscription.close()
		})
	}
}

func TestEventSubscription_EventSupported(t *testing.T) {
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
		t.Run(testCase.name, func(t *testing.T) {
			for _, eventType := range testCase.events {
				assert.Equal(t, testCase.supported, subscription.eventSupported(eventType))
			}
		})
	}
}
