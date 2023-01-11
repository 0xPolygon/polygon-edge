package types

import (
	"math/big"
	"sync/atomic"
)

// txBase is the base tx model used by each specific tx type.
// Contains common fields and functions.
type txBase struct {
	nonce    uint64
	gasPrice *big.Int
	gas      uint64
	to       *Address
	value    *big.Int
	input    []byte
	v, r, s  *big.Int
	hash     Hash
	from     Address

	tp TxType

	// Cache
	size atomic.Value
}

// Nonce returns transaction nonce.
func (t *txBase) Nonce() uint64 {
	return t.nonce
}

// GasPrice returns transaction gas price.
func (t *txBase) GasPrice() *big.Int {
	return new(big.Int).Set(t.gasPrice)
}

// Gas returns transaction gas.
func (t *txBase) Gas() uint64 {
	return t.gas
}

// To returns transaction receiver address.
// It could be nil in case of the state transaction.
func (t *txBase) To() *Address {
	return t.to.CopyPtr()
}

// Value returns transaction value.
func (t *txBase) Value() *big.Int {
	return new(big.Int).Set(t.value)
}

// Input returns transaction input value.
func (t *txBase) Input() []byte {
	return t.input
}

// Hash returns transaction hash.
func (t *txBase) Hash() Hash {
	return t.hash
}

// From returns transaction sender address.
func (t *txBase) From() Address {
	return t.from
}

// Signature returns transaction signature values.
func (t *txBase) Signature() (v *big.Int, r *big.Int, s *big.Int) {
	return new(big.Int).Set(t.v),
		new(big.Int).Set(t.r),
		new(big.Int).Set(t.s)
}

// Type returns the given transaction type.
// It could be a state transaction, legacy transaction, or
// dynamic gas one.
func (t *txBase) Type() TxType {
	return t.tp
}

// Cost returns gas * gasPrice + value.
// Same for all transaction types.
// Implements Tx interface.
func (t *txBase) Cost() *big.Int {
	total := new(big.Int).Mul(t.gasPrice, new(big.Int).SetUint64(t.gas))
	total.Add(total, t.value)

	return total
}

// copy makes a copy of the base tx model.
func (t *txBase) copy() *txBase {
	tt := new(txBase)
	*tt = *t

	tt.gasPrice = new(big.Int)
	if t.gasPrice != nil {
		tt.gasPrice.Set(t.gasPrice)
	}

	tt.value = new(big.Int)
	if t.value != nil {
		tt.value.Set(t.value)
	}

	if t.r != nil {
		tt.r = new(big.Int)
		tt.r = big.NewInt(0).SetBits(t.r.Bits())
	}

	if t.s != nil {
		tt.s = new(big.Int)
		tt.s = big.NewInt(0).SetBits(t.s.Bits())
	}

	tt.input = make([]byte, len(t.input))
	copy(tt.input[:], t.input[:])

	return tt
}

// getSize returns size of the transaction or 0
func (t *txBase) getSize() uint64 {
	if size := t.size.Load(); size != nil {
		sizeVal, ok := size.(uint64)
		if !ok {
			return 0
		}

		return sizeVal
	}

	return 0
}
