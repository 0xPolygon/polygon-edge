package jsonrpc

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestThrottling(t *testing.T) {
	t.Parallel()

	th := NewThrottling(5, time.Millisecond*100)
	sfn := func(value int, sleep time.Duration) func() (interface{}, error) {
		return func() (interface{}, error) {
			time.Sleep(sleep)

			return value, nil
		}
	}

	wg := sync.WaitGroup{}
	wg.Add(7)

	go func() {
		defer wg.Done()

		res, err := th.AttemptRequest(context.Background(), sfn(100, time.Millisecond*1200))

		require.NoError(t, err)
		assert.Equal(t, 100, res.(int)) // nolint
	}()

	time.Sleep(time.Millisecond * 200)

	for i := 2; i <= 5; i++ {
		go func() {
			defer wg.Done()

			res, err := th.AttemptRequest(context.Background(), sfn(100, time.Millisecond*1000))

			require.NoError(t, err)
			assert.Equal(t, 100, res.(int)) // nolint
		}()
	}

	time.Sleep(time.Millisecond * 200)

	go func() {
		defer wg.Done()

		res, err := th.AttemptRequest(context.Background(), sfn(100, time.Millisecond))

		require.ErrorIs(t, err, errRequestLimitExceeded)
		assert.Nil(t, res)
	}()

	time.Sleep(time.Millisecond * 1000)

	go func() {
		defer wg.Done()

		res, err := th.AttemptRequest(context.Background(), sfn(10, time.Millisecond))

		require.NoError(t, err)
		assert.Equal(t, 10, res.(int)) // nolint
	}()

	wg.Wait()
}
