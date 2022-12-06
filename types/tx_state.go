package types

import (
	"math/big"
)

// StateTx implements Transaction interface with the state tx logic
type StateTx struct {
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
func (tx *StateTx) Copy() TxData {
	tt := new(StateTx)
	*tt = *tx

	tt.GasPrice = new(big.Int)
	if tx.GasPrice != nil {
		tt.GasPrice.Set(tx.GasPrice)
	}

	tt.Value = new(big.Int)
	if tx.Value != nil {
		tt.Value.Set(tx.Value)
	}

	if tx.R != nil {
		tt.R = new(big.Int)
		tt.R = big.NewInt(0).SetBits(tx.R.Bits())
	}

	if tx.S != nil {
		tt.S = new(big.Int)
		tt.S = big.NewInt(0).SetBits(tx.S.Bits())
	}

	tt.Input = make([]byte, len(tx.Input))
	copy(tt.Input[:], tx.Input[:])

	return tt
}

// accessors for innerTx.
func (tx *StateTx) txType() TxType      { return StateTxType }
func (tx *StateTx) input() []byte       { return tx.Input }
func (tx *StateTx) gas() uint64         { return tx.Gas }
func (tx *StateTx) gasPrice() *big.Int  { return tx.GasPrice }
func (tx *StateTx) gasTipCap() *big.Int { return tx.GasPrice }
func (tx *StateTx) gasFeeCap() *big.Int { return tx.GasPrice }
func (tx *StateTx) value() *big.Int     { return tx.Value }
func (tx *StateTx) nonce() uint64       { return tx.Nonce }
func (tx *StateTx) from() Address       { return tx.From }
func (tx *StateTx) to() *Address        { return tx.To }

func (tx *StateTx) rawSignatureValues() (v, r, s *big.Int) {
	return tx.V, tx.R, tx.S
}

func (tx *StateTx) setSignatureValues(v, r, s *big.Int) {
	tx.V, tx.R, tx.S = v, r, s
}
