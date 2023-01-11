package types

import "math/big"

// TxType is the transaction type.
type TxType byte

// List of supported transaction types
const (
	LegacyTx     TxType = 0x0
	StateTx      TxType = 0x7f
	DynamicGeeTx TxType = 0x8f
)

// String returns string representation of the transaction type.
func (t TxType) String() (s string) {
	switch t {
	case LegacyTx:
		return "LegacyTx"
	case StateTx:
		return "StateTx"
	case DynamicGeeTx:
		return "DynamicGeeTx"
	default:
		return "UnknownTX"
	}
}

// Tx represents the general transaction behavior interface.
// It contains common transaction field getters and setters as
// well as additional functions for managing transaction.
// Any modification function should return modified transaction object.
type Tx interface {
	// Nonce returns transaction nonce.
	Nonce() uint64

	// SetNonce sets the given transaction nonce.
	SetNonce(nonce uint64) Tx

	// GasPrice returns transaction gas price.
	GasPrice() *big.Int

	// SetGasPrice sets the given transaction gas price.
	SetGasPrice(gasPrice *big.Int) Tx

	// Gas returns transaction gas.
	Gas() uint64

	// SetGas sets the given transaction gas.
	SetGas(gas uint64) Tx

	// To returns transaction receiver address.
	// It could be nil in case of the state transaction.
	To() *Address

	// SetTo sets the given transaction receiver address.
	SetTo(addr Address) Tx

	// Value returns transaction value.
	Value() *big.Int

	// SetValue sets the given transaction value.
	SetValue(value *big.Int) Tx

	// Input returns transaction input value.
	Input() []byte

	// SetInput sets the given transaction input value.
	SetInput(input []byte) Tx

	// Hash returns transaction hash.
	Hash() Hash

	// ComputeHash computes transaction hash based on the current tx values.
	ComputeHash() Tx

	// From returns transaction sender address.
	From() Address

	// SetFrom sets the given transaction sender address.
	SetFrom(from Address) Tx

	// Signature returns transaction signature values.
	Signature() (v *big.Int, r *big.Int, s *big.Int)

	// SetSignature sets the given signature values of the transaction.
	SetSignature(v *big.Int, r *big.Int, s *big.Int) Tx

	// Type returns the given transaction type.
	// It could be a state transaction, legacy transaction, or
	// dynamic gas one.
	Type() TxType

	// Cost calculates and returns the transaction cost.
	Cost() *big.Int

	// Size returns transaction size.
	Size() uint64

	// Copy of the current transaction and returns a new transaction model.
	Copy() Tx
}
