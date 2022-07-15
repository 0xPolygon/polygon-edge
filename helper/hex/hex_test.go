package hex

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestEncodingUint64(t *testing.T) {
	testTable := []struct {
		name       string
		value      uint64
		shouldFail bool
	}{
		{
			"Successfully encode and decode uint64",
			245,
			false,
		},
	}

	for _, testCase := range testTable {
		t.Run(testCase.name, func(t *testing.T) {
			encodedValue := EncodeUint64(testCase.value)

			decodedValue, err := DecodeUint64(encodedValue)
			if err != nil && !testCase.shouldFail {
				t.Fatalf("Unable to decode uint64, %v", err)
			}

			assert.Equal(t, testCase.value, decodedValue)

			decodedValue = MustDecodeUint64(encodedValue)

			assert.Equal(t, testCase.value, decodedValue)
		})
	}
}
