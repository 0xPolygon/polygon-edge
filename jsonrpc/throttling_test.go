package jsonrpc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestThrottling(t *testing.T) {
	t.Parallel()

	th := NewThrottling(5)

	assert.NoError(t, th.AttemptRequest())
	assert.NoError(t, th.AttemptRequest())

	time.Sleep(time.Millisecond * 100)

	assert.NoError(t, th.AttemptRequest())
	assert.NoError(t, th.AttemptRequest())

	time.Sleep(time.Millisecond * 100)

	assert.NoError(t, th.AttemptRequest())
	assert.ErrorIs(t, th.AttemptRequest(), errRequestLimitExceeded)

	time.Sleep(time.Millisecond * 100)

	assert.ErrorIs(t, th.AttemptRequest(), errRequestLimitExceeded)

	time.Sleep(time.Millisecond * 701)

	assert.NoError(t, th.AttemptRequest())
	assert.Equal(t, 4, th.requests.Len())

	time.Sleep(time.Millisecond * 1002)

	assert.NoError(t, th.AttemptRequest())
	assert.Equal(t, 1, th.requests.Len())
}
