package hex

import (
	"fmt"
	"math/big"
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

func TestEncodeToHex(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		input    []byte
		expected string
	}{
		{[]byte{}, "0x"},
		{[]byte{0x00}, "0x00"},
		{[]byte{0x01}, "0x01"},
		{[]byte{0x0A, 0x0B, 0x0C}, "0x0a0b0c"},
		{[]byte{0xFF, 0xFE, 0xFD}, "0xfffefd"},
	}

	for _, tc := range testCases {
		actual := EncodeToHex(tc.input)
		assert.Equal(t, tc.expected, actual)
	}
}

func TestEncodeToString(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		input    []byte
		expected string
	}{
		{[]byte{}, ""},
		{[]byte{0x00}, "00"},
		{[]byte{0x01}, "01"},
		{[]byte{0x0A, 0x0B, 0x0C}, "0a0b0c"},
		{[]byte{0xFF, 0xFE, 0xFD}, "fffefd"},
	}

	for _, tc := range testCases {
		actual := EncodeToString(tc.input)
		assert.Equal(t, tc.expected, actual)
	}
}

func TestMustDecodeHex(t *testing.T) {
	t.Parallel()

	t.Run("Valid hex string", func(t *testing.T) {
		t.Parallel()

		str := "0x48656c6c6f20576f726c64"
		expected := []byte{0x48, 0x65, 0x6c, 0x6c, 0x6f, 0x20, 0x57, 0x6f, 0x72, 0x6c, 0x64}

		actual := MustDecodeHex(str)
		assert.Equal(t, expected, actual)
	})

	t.Run("Empty hex string", func(t *testing.T) {
		t.Parallel()

		str := ""
		expected := []byte{}

		actual := MustDecodeHex(str)
		assert.Equal(t, expected, actual)
	})

	t.Run("Invalid hex string", func(t *testing.T) {
		t.Parallel()

		str := "0x12345q"

		require.PanicsWithError(t, "could not decode hex: encoding/hex: invalid byte: U+0071 'q'", func() {
			MustDecodeHex(str)
		})
	})
}

func TestEncodeUint64(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		input    uint64
		expected string
	}{
		{0, "0x0"},
		{1, "0x1"},
		{11, "0xb"},
		{67312, "0x106f0"},
		{80604, "0x13adc"},
		{^uint64(0), "0xffffffffffffffff"}, // max uint64
	}

	for _, tc := range testCases {
		actual := EncodeUint64(tc.input)
		assert.Equal(t, tc.expected, actual)
	}
}

func TestEncodeBig(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		input    *big.Int
		expected string
	}{
		{big.NewInt(0), "0x0"},
		{big.NewInt(1), "0x1"},
		{big.NewInt(11), "0xb"},
		{big.NewInt(67312), "0x106f0"},
		{big.NewInt(80604), "0x13adc"},
		{new(big.Int).SetUint64(^uint64(0)), "0xffffffffffffffff"}, // max uint64
	}

	for _, tc := range testCases {
		actual := EncodeBig(tc.input)
		assert.Equal(t, tc.expected, actual)
	}
}

func TestDecodeString(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		input    string
		expected []byte
	}{
		{"", []byte{}},
		{"48656c6c6f20576f726c64", []byte{0x48, 0x65, 0x6c, 0x6c, 0x6f, 0x20, 0x57, 0x6f, 0x72, 0x6c, 0x64}},
		{"0123456789abcdef", []byte{0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef}},
		{"fffe", []byte{0xff, 0xfe}},
	}

	for _, tc := range testCases {
		actual, err := DecodeString(tc.input)
		assert.NoError(t, err)
		assert.Equal(t, tc.expected, actual)
	}
}
