package network

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSlots(t *testing.T) {
	t.Parallel()

	slots := NewSlots(4)

	for i := 0; i < 4; i++ {
		slots.Release() // should do nothing
	}

	for i := 3; i >= 0; i-- {
		closed := slots.Take(context.Background())
		assert.False(t, closed)
	}

	go func() {
		time.Sleep(time.Millisecond * 500)
		slots.Release() // return one slot after 500 milis
		time.Sleep(time.Millisecond * 500)
		slots.Release() // return another slot after 1 seconds
	}()

	tm := time.Now().UTC()

	closed1 := slots.Take(context.Background())
	closed2 := slots.Take(context.Background())

	assert.False(t, closed1)
	assert.GreaterOrEqual(t, time.Now().UTC(), tm.Add(time.Millisecond*500))
	assert.False(t, closed2)
	assert.GreaterOrEqual(t, time.Now().UTC(), tm.Add(time.Millisecond*500*2))
}
