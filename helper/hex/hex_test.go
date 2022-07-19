package hex

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

const (
	minUint64 uint64 = 0
	maxUint64 uint64 = 18446744073709551615
)

func TestEncodingUint64(t *testing.T) {
	// random uint64 array, min and max included
	uint64Array := [10]uint64{minUint64, 30073, 67312, 71762, 11, 80604, 45403, 1, 298, maxUint64}

	for _, value := range uint64Array {
		encodedValue := EncodeUint64(value)

		decodedValue, err := DecodeUint64(encodedValue)
		assert.NoError(t, err)

		assert.Equal(t, value, decodedValue)
	}
}
