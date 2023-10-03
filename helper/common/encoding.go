package common

import (
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/0xPolygon/polygon-edge/helper/hex"
)

// ParseUint64orHex parses the given uint64 hex string into the number.
// It can parse the string with 0x prefix as well.
func ParseUint64orHex(val *string) (uint64, error) {
	if val == nil {
		return 0, nil
	}

	str := *val
	base := 10

	if strings.HasPrefix(str, "0x") {
		str = str[2:]
		base = 16
	}

	return strconv.ParseUint(str, base, 64)
}
func ParseUint256orHex(val *string) (*big.Int, error) {
	if val == nil {
		return nil, nil
	}

	str := *val
	base := 10

	if strings.HasPrefix(str, "0x") {
		str = str[2:]
		base = 16
	}

	b, ok := new(big.Int).SetString(str, base)
	if !ok {
		return nil, fmt.Errorf("could not parse")
	}

	return b, nil
}

func ParseBytes(val *string) ([]byte, error) {
	if val == nil {
		return []byte{}, nil
	}

	str := strings.TrimPrefix(*val, "0x")

	return hex.DecodeString(str)
}

func EncodeUint64(b uint64) *string {
	res := fmt.Sprintf("0x%x", b)

	return &res
}

func EncodeBytes(b []byte) *string {
	res := "0x" + hex.EncodeToString(b)

	return &res
}

func EncodeBigInt(b *big.Int) *string {
	res := "0x" + b.Text(16)

	return &res
}
