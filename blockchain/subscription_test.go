package blockchain

import (
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/types"
)

func TestSubscriptionLinear(t *testing.T) {
	e := &eventStream{}

	// add a genesis block to eventstream
	e.push(&Event{
		NewChain: []*types.Header{
			{Number: 0},
		},
	})

	sub := e.subscribe()

	eventCh := make(chan *Event)

	go func() {
		for {
			task := sub.GetEvent()
			eventCh <- task
		}
	}()

	for i := 1; i < 10; i++ {
		evnt := &Event{}

		evnt.AddNewHeader(&types.Header{Number: uint64(i)})
		e.push(evnt)

		// it should fire updateCh
		select {
		case evnt := <-eventCh:
			if evnt.NewChain[0].Number != uint64(i) {
				t.Fatal("bad")
			}
		case <-time.After(1 * time.Second):
			t.Fatal("timeout")
		}
	}
}

func TestSubscriptionSlowConsumer(t *testing.T) {
	e := &eventStream{}

	e.push(&Event{
		NewChain: []*types.Header{
			{Number: 0},
		},
	})

	sub := e.subscribe()

	// send multiple events
	for i := 1; i < 10; i++ {
		e.push(&Event{
			NewChain: []*types.Header{
				{Number: uint64(i)},
			},
		})
	}

	// consume events now
	for i := 1; i < 10; i++ {
		evnt := sub.GetEvent()
		if evnt.NewChain[0].Number != uint64(i) {
			t.Fatal("bad")
		}
	}
}
