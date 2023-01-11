package types

import "math/big"

// txLegacy implements Tx interface with the legacy transaction logic.
type txLegacy struct {
	txBase
}

// SetNonce sets the given transaction nonce.
func (t *txLegacy) SetNonce(nonce uint64) Tx {
	t.nonce = nonce

	return t
}

// SetGasPrice sets the given transaction gas price.
func (t *txLegacy) SetGasPrice(gasPrice *big.Int) Tx {
	t.gasPrice = gasPrice

	return t
}

// SetGas sets the given transaction gas.
func (t *txLegacy) SetGas(gas uint64) Tx {
	t.gas = gas

	return t
}

// SetTo sets the given transaction receiver address.
func (t *txLegacy) SetTo(addr Address) Tx {
	t.to = addr.Ptr()

	return t
}

// SetValue sets the given transaction value.
func (t *txLegacy) SetValue(value *big.Int) Tx {
	t.value = value

	return t
}

// SetInput sets the given transaction input value.
func (t *txLegacy) SetInput(input []byte) Tx {
	t.input = input

	return t
}

// SetFrom sets the given transaction sender address.
func (t *txLegacy) SetFrom(from Address) Tx {
	t.from = from

	return t
}

// SetSignature sets the given signature values of the transaction.
func (t *txLegacy) SetSignature(v *big.Int, r *big.Int, s *big.Int) Tx {
	t.v = v
	t.r = r
	t.s = s

	return t
}

// ComputeHash computes the hash of the transaction.
// Defined per specific transaction because hash computation
// logic could be different for tx type.
func (t *txLegacy) ComputeHash() Tx {
	panic("not implemented yet")
}

// Size returns transaction size.
func (t *txLegacy) Size() uint64 {
	if sz := t.getSize(); sz > 0 {
		return sz
	}

	size := uint64(0) // TODO: Computate size and set it
	t.size.Store(size)

	return size
}

// Copy of the current transaction and returns a new transaction model.
func (t *txLegacy) Copy() Tx {
	newTx := txLegacy{}
	newTx.txBase = *t.copy()

	return &newTx
}
