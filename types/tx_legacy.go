package types

import (
	"math/big"
)

// LegacyTx implements Transaction interface with the legacy tx logic
type LegacyTx struct {
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
func (t *LegacyTx) Copy() TxData {
	tt := new(LegacyTx)
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
func (t *LegacyTx) txType() TxType      { return LegacyTxType }
func (t *LegacyTx) input() []byte       { return t.Input }
func (t *LegacyTx) gas() uint64         { return t.Gas }
func (t *LegacyTx) gasPrice() *big.Int  { return t.GasPrice }
func (t *LegacyTx) gasTipCap() *big.Int { return t.GasPrice }
func (t *LegacyTx) gasFeeCap() *big.Int { return t.GasPrice }
func (t *LegacyTx) value() *big.Int     { return t.Value }
func (t *LegacyTx) nonce() uint64       { return t.Nonce }
func (t *LegacyTx) from() Address       { return t.From }
func (t *LegacyTx) to() *Address        { return t.To }

func (t *LegacyTx) rawSignatureValues() (v, r, s *big.Int) {
	return t.V, t.R, t.S
}

func (t *LegacyTx) setSignatureValues(v, r, s *big.Int) {
	t.V, t.R, t.S = v, r, s
}
