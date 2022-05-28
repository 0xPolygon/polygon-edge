package jsonrpc

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

type ArgBig big.Int

func (a *ArgBig) UnmarshalText(input []byte) error {
	buf, err := parseHexBytes(string(input))
	if err != nil {
		return err
	}
	b := new(big.Int)
	b.SetBytes(buf)
	*a = ArgBig(*b)
	return nil
}

func (a *ArgBig) Big() *big.Int {
	b := big.Int(*a)
	return &b
}

func encodeUintToHex(i uint64) string {
	return fmt.Sprintf("0x%x", i)
}

func parseBigInt(str string) *big.Int {
	str = strings.TrimPrefix(str, "0x")
	num := new(big.Int)
	num.SetString(str, 16)
	return num
}

func parseUint64orHex(str string) (uint64, error) {
	base := 10
	if strings.HasPrefix(str, "0x") {
		str = str[2:]
		base = 16
	}
	return strconv.ParseUint(str, base, 64)
}

func encodeToHex(b []byte) string {
	return "0x" + hex.EncodeToString(b)
}

func parseHexBytes(str string) ([]byte, error) {
	if !strings.HasPrefix(str, "0x") {
		return nil, fmt.Errorf("it does not have 0x prefix")
	}
	str = strings.TrimPrefix(str, "0x")
	if len(str)%2 != 0 {
		str = "0" + str
	}
	buf, err := hex.DecodeString(str)
	if err != nil {
		return nil, err
	}
	return buf, nil
}
