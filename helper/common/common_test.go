package common

import (
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
