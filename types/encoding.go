package types

import (
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/umbracle/minimal/helper/hex"
)

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

func ParseInt64orHex(val *string) (int64, error) {
	i, err := ParseUint64orHex(val)
	return int64(i), err
}

func ParseBytes(val *string) ([]byte, error) {
	if val == nil {
		return []byte{}, nil
	}

	str := *val
	if strings.HasPrefix(str, "0x") {
		str = str[2:]
	}
	return hex.DecodeString(str)
}
