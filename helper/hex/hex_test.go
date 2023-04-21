package hex

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDecodeUint64 verifies that uint64 values
// are properly decoded from hex
func TestDecodeUint64(t *testing.T) {
	t.Parallel()

	uint64Array := []uint64{
		0,
		1,
		11,
		67312,
		80604,
		^uint64(0), // max uint64
	}

	toHexArr := func(nums []uint64) []string {
		numbers := make([]string, len(nums))

		for index, num := range nums {
			numbers[index] = fmt.Sprintf("0x%x", num)
		}

		return numbers
	}

	for index, value := range toHexArr(uint64Array) {
		decodedValue, err := DecodeUint64(value)
		assert.NoError(t, err)

		assert.Equal(t, uint64Array[index], decodedValue)
	}
}

func TestDecodeHexToBig(t *testing.T) {
	t.Parallel()

	t.Run("Error on decoding", func(t *testing.T) {
		t.Parallel()

		value, err := DecodeHexToBig("012345q")
		require.ErrorContains(t, err, "failed to convert string")
		require.Nil(t, value)
	})

	t.Run("Happy path", func(t *testing.T) {
		t.Parallel()

		big, err := DecodeHexToBig("0x0123456")
		require.NoError(t, err)
		require.Equal(t, uint64(1193046), big.Uint64())

		big, err = DecodeHexToBig("0123456") // this should work as well
		require.NoError(t, err)
		require.Equal(t, uint64(1193046), big.Uint64())

		big, err = DecodeHexToBig("0x0000000000000000000000000000000000000200")
		require.NoError(t, err)
		require.Equal(t, uint64(512), big.Uint64())
	})
}
