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

	assert.Equal(t, int64(4), slots.GetAvailableCount())

	num := slots.ReturnSlot() // should do nothing

	assert.Equal(t, int64(4), num)
	assert.Equal(t, int64(4), slots.GetAvailableCount())

	for i := 3; i >= 0; i-- {
		num, closed := slots.TakeSlot(context.Background())

		assert.False(t, closed)
		assert.Equal(t, int64(i), num)
	}

	go func() {
		<-time.After(time.Second * 1)

		_ = slots.ReturnSlot() // return one slot after 2 seconds
	}()

	tm := time.Now().UTC()

	num, closed := slots.TakeSlot(context.Background())

	assert.False(t, closed)
	assert.Equal(t, int64(0), num)
	assert.GreaterOrEqual(t, time.Now().UTC(), tm.Add(time.Second*1))
}
