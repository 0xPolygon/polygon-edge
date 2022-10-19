package blockchain

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/goleak"

	"github.com/0xPolygon/polygon-edge/types"
)

func TestSubscription(t *testing.T) {
	t.Parallel()
	defer goleak.VerifyNone(t)

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
