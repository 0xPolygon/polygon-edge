package types

import (
	"math/big"
)

// StateTx implements Transaction interface with the state tx logic
type StateTx struct {
	Nonce    uint64
	GasPrice *big.Int
	Gas      uint64
	To       *Address
	Value    *big.Int
	Input    []byte
	V, R, S  *big.Int
}

// Copy creates a deep copy of the transaction data and initializes all fields.
func (t *StateTx) Copy() TxData {
	tt := new(StateTx)
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
func (t *StateTx) txType() TxType      { return StateTxType }
func (t *StateTx) input() []byte       { return t.Input }
func (t *StateTx) gas() uint64         { return t.Gas }
func (t *StateTx) setGas(gas uint64)   { t.Gas = gas }
func (t *StateTx) gasPrice() *big.Int  { return t.GasPrice }
func (t *StateTx) gasTipCap() *big.Int { return t.GasPrice }
func (t *StateTx) gasFeeCap() *big.Int { return t.GasPrice }
func (t *StateTx) value() *big.Int     { return t.Value }
func (t *StateTx) nonce() uint64       { return t.Nonce }
func (t *StateTx) to() *Address        { return t.To }

func (t *StateTx) rawSignatureValues() (v, r, s *big.Int) {
	return t.V, t.R, t.S
}

func (t *StateTx) setSignatureValues(v, r, s *big.Int) {
	t.V, t.R, t.S = v, r, s
}
