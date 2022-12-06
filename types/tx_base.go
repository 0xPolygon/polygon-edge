package types

import (
	"math/big"
)

type baseTx struct {
	Nonce    uint64
	GasPrice *big.Int
	Gas      uint64
	From     Address
	To       *Address
	Value    *big.Int
	Input    []byte
	V, R, S  *big.Int
}

// Copy creates a deep copy of the transaction data and initializes all fields.
func (t *baseTx) copy() *baseTx {
	tt := new(baseTx)
	*tt = *t

	tt.GasPrice = new(big.Int)
	if t.GasPrice != nil {
		tt.GasPrice.Set(t.GasPrice)
	}

	tt.Value = new(big.Int)
	if t.Value != nil {
		tt.Value.Set(t.Value)
	}

	if t.R != nil {
		tt.R = new(big.Int)
		tt.R = big.NewInt(0).SetBits(t.R.Bits())
	}

	if t.S != nil {
		tt.S = new(big.Int)
		tt.S = big.NewInt(0).SetBits(t.S.Bits())
	}

	tt.Input = make([]byte, len(t.Input))
	copy(tt.Input[:], t.Input[:])

	return tt
}

// accessors for innert.
func (t *baseTx) input() []byte       { return t.Input }
func (t *baseTx) gas() uint64         { return t.Gas }
func (t *baseTx) gasPrice() *big.Int  { return t.GasPrice }
func (t *baseTx) gasTipCap() *big.Int { return t.GasPrice }
func (t *baseTx) gasFeeCap() *big.Int { return t.GasPrice }
func (t *baseTx) value() *big.Int     { return t.Value }
func (t *baseTx) nonce() uint64       { return t.Nonce }
func (t *baseTx) from() Address       { return t.From }
func (t *baseTx) to() *Address        { return t.To }

func (t *baseTx) rawSignatureValues() (v, r, s *big.Int) {
	return t.V, t.R, t.S
}

func (t *baseTx) setSignatureValues(v, r, s *big.Int) {
	t.V, t.R, t.S = v, r, s
}
