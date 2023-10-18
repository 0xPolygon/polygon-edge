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
		e              = newEventStream()
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

	defer e.unsubscribe(sub)

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
		e                  = newEventStream()
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
		e.push(&Event{})
	}

	// Wait for the events to be processed
	wg.Wait()

	for _, s := range subscriptions {
		e.unsubscribe(s)
	}
}

func TestSubscription_AfterOneUnsubscribe(t *testing.T) {
	t.Parallel()

	var (
		e = newEventStream()

		wg sync.WaitGroup
	)

	sub1 := e.subscribe()
	sub2 := e.subscribe()

	wg.Add(2)

	worker := func(sub *subscription, expectedBlockCount uint8) {
		defer wg.Done()

		updateCh := sub.GetEventCh()
		receivedBlockCount := uint8(0)

		for {
			select {
			case <-updateCh:
				receivedBlockCount++
				if receivedBlockCount == expectedBlockCount {
					e.unsubscribe(sub)

					return
				}
			case <-time.After(10 * time.Second):
				e.unsubscribe(sub)
				t.Errorf("subscription did not caught all events")
			}
		}
	}

	go worker(sub1, 10)
	go worker(sub2, 20)

	// Send the events to the channels
	for i := 0; i < 20; i++ {
		e.push(&Event{})
		time.Sleep(time.Millisecond)
	}

	// Wait for the event to be parsed
	wg.Wait()
}

func TestSubscription_NilEventAfterClosingSubscription(t *testing.T) {
	t.Parallel()

	var (
		e = newEventStream()

		wg sync.WaitGroup
	)

	sub := e.subscribe()

	wg.Add(1)

	receivedEvtCount := 0

	worker := func(sub *subscription, expectedBlockCount uint8) {
		defer wg.Done()

		startTime := time.Now().UTC()
		timeout := 5 * time.Second

		for {
			evt := sub.GetEvent()
			if evt != nil {
				receivedEvtCount++
			} else {
				return
			}

			// Check for timeout
			if time.Since(startTime) >= timeout {
				t.Errorf("subscription did not caught all events")

				return
			}
		}
	}

	go worker(sub, 2)

	// Send the events to the channels
	for i := 0; i < 2; i++ {
		e.push(&Event{})
		time.Sleep(time.Millisecond)
	}

	e.unsubscribe(sub)

	// Wait for the event to be parsed
	wg.Wait()

	assert.Equal(t, 2, receivedEvtCount)
}
