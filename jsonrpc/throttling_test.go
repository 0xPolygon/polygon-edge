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

	th := NewThrottling(5, time.Millisecond*50)
	sfn := func(value int, sleep time.Duration) func() (interface{}, error) {
		return func() (interface{}, error) {
			time.Sleep(sleep)

			return value, nil
		}
	}

	wg := sync.WaitGroup{}
	wg.Add(9)

	go func() {
		defer wg.Done()

		res, err := th.AttemptRequest(context.Background(), sfn(100, time.Millisecond*500))

		require.NoError(t, err)
		assert.Equal(t, 100, res.(int)) //nolint
	}()

	time.Sleep(time.Millisecond * 100)

	for i := 2; i <= 5; i++ {
		go func() {
			defer wg.Done()

			res, err := th.AttemptRequest(context.Background(), sfn(100, time.Millisecond*1000))

			require.NoError(t, err)
			assert.Equal(t, 100, res.(int)) //nolint
		}()
	}

	go func() {
		time.Sleep(time.Millisecond * 150)

		defer wg.Done()

		res, err := th.AttemptRequest(context.Background(), sfn(100, time.Millisecond*100))

		require.ErrorIs(t, err, errRequestLimitExceeded)
		assert.Nil(t, res)
	}()

	go func() {
		time.Sleep(time.Millisecond * 620)

		defer wg.Done()

		res, err := th.AttemptRequest(context.Background(), sfn(10, time.Millisecond*100))

		require.NoError(t, err)
		assert.Equal(t, 10, res.(int)) //nolint
	}()

	go func() {
		time.Sleep(time.Millisecond * 640)

		defer wg.Done()

		res, err := th.AttemptRequest(context.Background(), sfn(100, time.Millisecond*100))

		require.ErrorIs(t, err, errRequestLimitExceeded)
		assert.Nil(t, res)
	}()

	go func() {
		time.Sleep(time.Millisecond * 1000)

		defer wg.Done()

		res, err := th.AttemptRequest(context.Background(), sfn(1, time.Millisecond*100))

		require.NoError(t, err)
		assert.Equal(t, 1, res.(int)) //nolint
	}()

	wg.Wait()
}
