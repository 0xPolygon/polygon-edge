package types

import (
	"fmt"

	"github.com/centrifuge/go-substrate-rpc-client/scale"
)

type ElectionCompute byte

const (
	// Result was forcefully computed on chain at the end of the session.
	OnChain ElectionCompute = 0
	// Result was submitted and accepted to the chain via a signed transaction.
	Signed ElectionCompute = 1
	// Result was submitted and accepted to the chain via an unsigned transaction (by an authority).
	Unsigned ElectionCompute = 2
)

func (ec *ElectionCompute) Decode(decoder scale.Decoder) error {
	b, err := decoder.ReadOneByte()
	vb := ElectionCompute(b)
	switch vb {
	case OnChain, Signed, Unsigned:
		*ec = vb
	default:
		return fmt.Errorf("unknown ElectionCompute enum: %v", vb)
	}
	return err
}

func (ec ElectionCompute) Encode(encoder scale.Encoder) error {
	return encoder.PushByte(byte(ec))
}
