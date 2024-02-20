package hex

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

// EncodeToHex generates a hex string based on the byte representation, with the '0x' prefix
func EncodeToHex(str []byte) string {
	return "0x" + hex.EncodeToString(str)
}

// EncodeToString is a wrapper method for hex.EncodeToString
func EncodeToString(str []byte) string {
	return hex.EncodeToString(str)
}

// DecodeString returns the byte representation of the hexadecimal string
func DecodeString(str string) ([]byte, error) {
	return hex.DecodeString(str)
}

// DecodeHex converts a hex string to a byte array
func DecodeHex(str string) ([]byte, error) {
	str = strings.TrimPrefix(str, "0x")

	return hex.DecodeString(str)
}

// MustDecodeHex type-checks and converts a hex string to a byte array
func MustDecodeHex(str string) []byte {
	buf, err := DecodeHex(str)
	if err != nil {
		panic(fmt.Errorf("could not decode hex: %w", err)) //nolint:gocritic
	}

	return buf
}

// EncodeUint64 encodes a number as a hex string with 0x prefix.
func EncodeUint64(i uint64) string {
	enc := make([]byte, 2, 10)
	copy(enc, "0x")

	return string(strconv.AppendUint(enc, i, 16))
}

// DecodeUint64 decodes a hex string with 0x prefix to uint64
func DecodeUint64(hexStr string) (uint64, error) {
	// remove 0x suffix if found in the input string
	cleaned := strings.TrimPrefix(hexStr, "0x")

	return strconv.ParseUint(cleaned, 16, 64)
}

// EncodeBig encodes bigint as a hex string with 0x prefix.
// The sign of the integer is ignored.
func EncodeBig(bigint *big.Int) string {
	if bigint.BitLen() == 0 {
		return "0x0"
	}

	return fmt.Sprintf("%#x", bigint)
}

// DecodeHexToBig converts a hex number to a big.Int value
func DecodeHexToBig(hexNum string) (*big.Int, error) {
	cleaned := strings.TrimPrefix(hexNum, "0x")

	value, ok := new(big.Int).SetString(cleaned, 16)
	if !ok {
		return nil, fmt.Errorf("failed to convert string: %s to big.Int with base: 16", hexNum)
	}

	return value, nil
}
