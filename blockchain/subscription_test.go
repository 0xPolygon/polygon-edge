package blockchain

import (
	"sync"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
)

func TestSubscription(t *testing.T) {
	t.Parallel()

	var (
		e              = &eventStream{}
		sub            = e.subscribe()
		caughtEventNum = uint64(0)
		event          = &Event{
			NewChain: []*types.Header{
				{
					Number: 100,
				},
			},
		}

		wg sync.WaitGroup
	)

	defer sub.Close()

	updateCh := sub.GetEventCh()

	wg.Add(1)

	go func() {
		defer wg.Done()

		select {
		case ev := <-updateCh:
			caughtEventNum = ev.NewChain[0].Number
		case <-time.After(5 * time.Second):
		}
	}()

	// Send the event to the channel
	e.push(event)

	// Wait for the event to be parsed
	wg.Wait()

	assert.Equal(t, event.NewChain[0].Number, caughtEventNum)
}

func TestSubscription_BufferedChannel_MultipleSubscriptions(t *testing.T) {
	t.Parallel()

	var (
		e                  = &eventStream{}
		wg                 sync.WaitGroup
		numOfEvents        = 100000
		numOfSubscriptions = 10
	)

	subscriptions := make([]*subscription, numOfSubscriptions)
	wg.Add(numOfSubscriptions)

	worker := func(id int, sub *subscription) {
		updateCh := sub.GetEventCh()
		caughtEvents := 0

		defer wg.Done()

		for {
			select {
			case <-updateCh:
				caughtEvents++
				if caughtEvents == numOfEvents {
					return
				}
			case <-time.After(10 * time.Second):
				t.Errorf("subscription %d did not caught all events", id)
			}
		}
	}

	for i := 0; i < numOfSubscriptions; i++ {
		sub := e.subscribe()
		subscriptions[i] = sub

		go worker(i, sub)
	}

	// Send the events to the channels
	for i := 0; i < numOfEvents; i++ {
		e.push(&Event{
			NewChain: []*types.Header{
				{
					Number: uint64(i),
				},
			},
		})
	}

	// Wait for the events to be processed
	wg.Wait()

	for _, s := range subscriptions {
		s.Close()
	}
}
