package jsonrpc

import (
	"math/big"
	"strconv"
	"strings"

	"github.com/0xPolygon/minimal/helper/hex"
	"github.com/0xPolygon/minimal/types"
)

type argBig big.Int

func argBigPtr(b *big.Int) *argBig {
	v := argBig(*b)
	return &v
}

func (a argBig) MarshalText() ([]byte, error) {
	b := (*big.Int)(&a)
	return []byte("0x" + b.Text(16)), nil
}

func argAddrPtr(a types.Address) *types.Address {
	return &a
}

type argUint64 uint64

func argUintPtr(n uint64) *argUint64 {
	v := argUint64(n)
	return &v
}

func (b argUint64) MarshalText() ([]byte, error) {
	buf := make([]byte, 2, 10)
	copy(buf, `0x`)
	buf = strconv.AppendUint(buf, uint64(b), 16)
	return buf, nil
}

func (u *argUint64) UnmarshalText(input []byte) error {
	str := strings.TrimPrefix(string(input), "0x")
	num, err := strconv.ParseUint(str, 16, 64)
	if err != nil {
		return err
	}
	*u = argUint64(num)
	return nil
}

type argBytes []byte

func argBytesPtr(b []byte) *argBytes {
	bb := argBytes(b)
	return &bb
}

func (b argBytes) MarshalText() ([]byte, error) {
	return []byte(hex.EncodeToHex(b)), nil
}

func (b *argBytes) UnmarshalText(input []byte) error {
	hh := hex.MustDecodeHex(string(input))
	aux := make([]byte, len(hh))
	copy(aux[:], hh[:])
	*b = aux
	return nil
}

// txnArgs is the transaction argument for the rpc endpoints
type txnArgs struct {
	From     *types.Address
	To       *types.Address
	Gas      *argUint64
	GasPrice *argBytes
	Value    *argBytes
	Input    *argBytes
	Nonce    *argUint64
}
