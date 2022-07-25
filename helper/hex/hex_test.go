package hex

import (
	"crypto/rand"
	"github.com/stretchr/testify/assert"
	"math/big"
	"testing"
)

// TestEncodeDecodeUint64 verifies that uint64 values
// are properly encoded and decoded from hex
func TestEncodeDecodeUint64(t *testing.T) {
	t.Parallel()

	generateRandomNumbers := func(count int) []uint64 {
		numbers := make([]uint64, count)

		for index := 0; index < count; index++ {
			randNum, _ := rand.Int(
				rand.Reader,
				big.NewInt(int64(count)),
			)

			numbers[index] = randNum.Uint64()
		}

		return numbers
	}

	for _, value := range generateRandomNumbers(100) {
		encodedValue := EncodeUint64(value)

		decodedValue, err := DecodeUint64(encodedValue)
		assert.NoError(t, err)

		assert.Equal(t, value, decodedValue)
	}
}
