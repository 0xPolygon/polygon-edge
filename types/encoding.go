package types

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/0xPolygon/polygon-edge/helper/hex"
)

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
