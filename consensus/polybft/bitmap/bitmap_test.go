package bitmap

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBitmap_Set(t *testing.T) {
	t.Parallel()

	data := []int{8, 15, 0, 7, 31, 60, 112, 7, 16, 241, 189, 60, 0, 19, 14, 25}
	unique := map[int]struct{}{}

	b := Bitmap{}

	for _, v := range data {
		unique[v] = struct{}{}

		b.Set(uint64(v))
	}

	// check if only values from data are populated
	for _, v := range data {
		require.True(t, b.IsSet(uint64(v)))
	}

	cntSet := 0

	for i := uint64(0); i < b.Len(); i++ {
		if _, exists := unique[int(i)]; exists {
			cntSet++
		}
	}

	require.Equal(t, len(unique), cntSet)
}

func TestBitmap_Extend(t *testing.T) {
	t.Parallel()

	data := []struct {
		index  uint64
		length uint64
	}{
		{0, 1},
		{8, 2},
		{15, 2},
		{30, 4},
		{17, 4},
		{120, 16},
		{39, 16},
		{277, 35},
		{8, 35},
	}
	b := Bitmap{}

	for _, dt := range data {
		b.Set(dt.index)
		require.True(t, b.Len() == dt.length*8, "assertion failed when setting index %d", dt.index)
		require.Len(t, b, int(dt.length))
	}
}
