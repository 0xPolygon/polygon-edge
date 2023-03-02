package hex

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
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

func TestDropHexPrefix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		input          []byte
		expectedOutput string
	}{
		{
			name:           "hex prefix should be removed",
			input:          []byte("0x123"),
			expectedOutput: "123",
		},
		{
			name:           "hex prefix should be removed(upper case)",
			input:          []byte("0X123"),
			expectedOutput: "123",
		},
		{
			name:           "no hex prefix",
			input:          []byte("abc"),
			expectedOutput: "abc",
		},
		{
			name:           "length less then expexted",
			input:          []byte("3"),
			expectedOutput: "3",
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			output := DropHexPrefix(test.input)
			assert.Equal(t, test.expectedOutput, string(output))
		})
	}
}
