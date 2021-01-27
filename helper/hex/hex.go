package hex

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

var (
	ErrSyntax        = &DecError{"invalid hex string"}
	ErrMissingPrefix = &DecError{"hex string without 0x prefix"}
	ErrEmptyNumber   = &DecError{"hex string \"0x\""}
	ErrLeadingZero   = &DecError{"hex number with leading zero digits"}
	ErrUint64Range   = &DecError{"hex number > 64 bits"}
	ErrBig256Range   = &DecError{"hex number > 256 bits"}
)

type DecError struct{ msg string }

func (err DecError) Error() string { return err.msg }

func EncodeToHex(str []byte) string {
	return "0x" + hex.EncodeToString(str)
}

func EncodeToString(str []byte) string {
	return hex.EncodeToString(str)
}

func DecodeString(str string) ([]byte, error) {
	return hex.DecodeString(str)
}

func MustDecodeString(str string) []byte {
	buf, err := DecodeString(str)
	if err != nil {
		panic(fmt.Errorf("could not decode string: %v", err))
	}
	return buf
}

func DecodeHex(str string) ([]byte, error) {
	if strings.HasPrefix(str, "0x") {
		str = str[2:]
	}
	return hex.DecodeString(str)
}

func MustDecodeHex(str string) []byte {
	buf, err := DecodeHex(str)
	if err != nil {
		panic(fmt.Errorf("could not decode hex: %v", err))
	}
	return buf
}

// EncodeUint64 encodes i as a hex string with 0x prefix.
func EncodeUint64(i uint64) string {
	enc := make([]byte, 2, 10)
	copy(enc, "0x")
	return string(strconv.AppendUint(enc, i, 16))
}

const BadNibble = ^uint64(0)

func DecodeNibble(in byte) uint64 {
	switch {
	case in >= '0' && in <= '9':
		return uint64(in - '0')
	case in >= 'A' && in <= 'F':
		return uint64(in - 'A' + 10)
	case in >= 'a' && in <= 'f':
		return uint64(in - 'a' + 10)
	default:
		return BadNibble
	}
}

// EncodeBig encodes bigint as a hex string with 0x prefix.
// The sign of the integer is ignored.
func EncodeBig(bigint *big.Int) string {
	nbits := bigint.BitLen()
	if nbits == 0 {
		return "0x0"
	}
	return fmt.Sprintf("%#x", bigint)
}

// DecodeHexToBig converts a hex number to a big.Int value
func DecodeHexToBig(hexNum string) *big.Int {
	createdNum := new(big.Int)
	createdNum.SetString(hexNum, 16)

	return createdNum
}
