package common

import (
	"math/big"
	"testing"

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
