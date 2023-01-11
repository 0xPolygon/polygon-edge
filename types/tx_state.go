package types

import "math/big"

// txState implements Tx interface with the state transaction logic.
type txState struct {
	txBase
}

// SetNonce sets the given transaction nonce.
func (t *txState) SetNonce(nonce uint64) Tx {
	t.nonce = nonce

	return t
}

// SetGasPrice sets the given transaction gas price.
func (t *txState) SetGasPrice(gasPrice *big.Int) Tx {
	t.gasPrice = new(big.Int).Set(gasPrice)

	return t
}

// SetGas sets the given transaction gas.
func (t *txState) SetGas(gas uint64) Tx {
	t.gas = gas

	return t
}

// SetTo sets the given transaction receiver address.
func (t *txState) SetTo(addr Address) Tx {
	t.to = addr.Ptr()

	return t
}

// SetValue sets the given transaction value.
func (t *txState) SetValue(value *big.Int) Tx {
	t.value = new(big.Int).Set(value)

	return t
}

// SetInput sets the given transaction input value.
func (t *txState) SetInput(input []byte) Tx {
	t.input = input

	return t
}

// SetFrom sets the given transaction sender address.
func (t *txState) SetFrom(from Address) Tx {
	t.from = from

	return t
}

// SetSignature sets the given signature values of the transaction.
func (t *txState) SetSignature(v *big.Int, r *big.Int, s *big.Int) Tx {
	t.v = new(big.Int).Set(v)
	t.r = new(big.Int).Set(r)
	t.s = new(big.Int).Set(s)

	return t
}

// ComputeHash computes the hash of the transaction.
// Defined per specific transaction because hash computation
// logic could be different for tx type.
func (t *txState) ComputeHash() Tx {
	panic("not implemented yet")
}

// Size returns transaction size.
func (t *txState) Size() uint64 {
	if sz := t.getSize(); sz > 0 {
		return sz
	}

	size := uint64(0) // TODO: Computate size and set it
	t.size.Store(size)

	return size
}

// Copy of the current transaction and returns a new transaction model.
func (t *txState) Copy() Tx {
	newTx := txState{}
	newTx.txBase = *t.copy()

	return &newTx
}
