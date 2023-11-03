package common

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func Test_ExtendByteSlice(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		length    int
		newLength int
	}{
		{"With trimming", 4, 2},
		{"Without trimming", 4, 8},
		{"Without trimming (same lengths)", 4, 4},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			originalSlice := make([]byte, c.length)
			for i := 0; i < c.length; i++ {
				originalSlice[i] = byte(i * 2)
			}

			newSlice := ExtendByteSlice(originalSlice, c.newLength)
			require.Len(t, newSlice, c.newLength)
			if c.length > c.newLength {
				require.Equal(t, originalSlice[:c.newLength], newSlice)
			} else {
				require.Equal(t, originalSlice, newSlice[:c.length])
			}
		})
	}
}

func Test_BigIntDivCeil(t *testing.T) {
	t.Parallel()

	cases := []struct {
		a      int64
		b      int64
		result int64
	}{
		{a: 10, b: 3, result: 4},
		{a: 13, b: 6, result: 3},
		{a: -1, b: 3, result: 0},
		{a: -20, b: 3, result: -6},
		{a: -15, b: 3, result: -5},
	}

	for _, c := range cases {
		require.Equal(t, c.result, BigIntDivCeil(big.NewInt(c.a), big.NewInt(c.b)).Int64())
	}
}

func Test_Duration_Marshal_UnmarshalJSON(t *testing.T) {
	t.Parallel()

	t.Run("use duration standalone", func(t *testing.T) {
		t.Parallel()

		original := &Duration{Duration: 5 * time.Minute}
		originalRaw, err := original.MarshalJSON()
		require.NoError(t, err)

		other := &Duration{}
		require.NoError(t, other.UnmarshalJSON(originalRaw))
		require.Equal(t, original, other)
	})

	t.Run("use duration in wrapper struct", func(t *testing.T) {
		t.Parallel()

		type timer struct {
			Elapsed Duration `json:"elapsed"`
		}

		dur, err := time.ParseDuration("2h35m21s")
		require.NoError(t, err)

		origTimer := &timer{Elapsed: Duration{dur}}

		timerRaw, err := json.Marshal(origTimer)
		require.NoError(t, err)

		var otherTimer *timer
		require.NoError(t, json.Unmarshal(timerRaw, &otherTimer))
		require.Equal(t, origTimer, otherTimer)
	})
}

func TestRetryForever_AlwaysReturnError_ShouldNeverEnd(t *testing.T) {
	interval := time.Millisecond * 10
	ended := false

	go func() {
		RetryForever(context.Background(), interval, func(ctx context.Context) error {
			return errors.New("")
		})

		ended = true
	}()
	time.Sleep(interval * 10)
	require.False(t, ended)
}

func TestRetryForever_CancelContext_ShouldEnd(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	cancel()
	RetryForever(ctx, time.Millisecond*100, func(ctx context.Context) error {
		return errors.New("")
	})
	require.True(t, errors.Is(ctx.Err(), context.Canceled))
}

func Test_SafeAddUint64(t *testing.T) {
	cases := []struct {
		a        uint64
		b        uint64
		result   uint64
		overflow bool
	}{
		{
			a:      10,
			b:      4,
			result: 14,
		},
		{
			a:      0,
			b:      5,
			result: 5,
		},
		{
			a:        math.MaxUint64,
			b:        3,
			result:   0,
			overflow: true,
		},
		{
			a:        math.MaxUint64,
			b:        math.MaxUint64,
			result:   0,
			overflow: true,
		},
	}

	for i, c := range cases {
		t.Run(fmt.Sprintf("%d case", i+1), func(t *testing.T) {
			actualResult, actualOverflow := SafeAddUint64(c.a, c.b)
			require.Equal(t, c.result, actualResult)
			require.Equal(t, c.overflow, actualOverflow)
		})
	}
}
