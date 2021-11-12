package types

import (
	"fmt"

	"github.com/centrifuge/go-substrate-rpc-client/scale"
)

type BalanceStatus byte

const (
	// Funds are free, as corresponding to `free` item in Balances.
	Free BalanceStatus = 0
	// Funds are reserved, as corresponding to `reserved` item in Balances.
	Reserved BalanceStatus = 1
)

func (bs *BalanceStatus) Decode(decoder scale.Decoder) error {
	b, err := decoder.ReadOneByte()
	vb := BalanceStatus(b)
	switch vb {
	case Free, Reserved:
		*bs = vb
	default:
		return fmt.Errorf("unknown BalanceStatus enum: %v", vb)
	}
	return err
}

func (bs BalanceStatus) Encode(encoder scale.Encoder) error {
	return encoder.PushByte(byte(bs))
}
