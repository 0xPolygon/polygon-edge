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

// copy creates a deep copy of the transaction data and initializes all fields.
func (tx *LegacyTx) copy() TxData {
	tt := new(LegacyTx)
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
func (tx *LegacyTx) txType() TxType      { return LegacyTxType }
func (tx *LegacyTx) input() []byte       { return tx.Input }
func (tx *LegacyTx) gas() uint64         { return tx.Gas }
func (tx *LegacyTx) gasPrice() *big.Int  { return tx.GasPrice }
func (tx *LegacyTx) gasTipCap() *big.Int { return tx.GasPrice }
func (tx *LegacyTx) gasFeeCap() *big.Int { return tx.GasPrice }
func (tx *LegacyTx) value() *big.Int     { return tx.Value }
func (tx *LegacyTx) nonce() uint64       { return tx.Nonce }
func (tx *LegacyTx) from() Address       { return tx.From }
func (tx *LegacyTx) to() *Address        { return tx.To }

func (tx *LegacyTx) rawSignatureValues() (v, r, s *big.Int) {
	return tx.V, tx.R, tx.S
}

func (tx *LegacyTx) setSignatureValues(v, r, s *big.Int) {
	tx.V, tx.R, tx.S = v, r, s
}
