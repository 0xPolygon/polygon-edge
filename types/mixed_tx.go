package types

import "math/big"

type MixedTxn struct {
	Nonce     uint64
	GasPrice  *big.Int
	GasTipCap *big.Int
	GasFeeCap *big.Int
	Gas       uint64
	To        *Address
	Value     *big.Int
	Input     []byte
	V, R, S   *big.Int
	Hash      Hash
	From      Address

	Type TxType

	ChainID    *big.Int
	AccessList TxAccessList
}

func (tx *MixedTxn) transactionType() TxType { return tx.Type }
func (tx *MixedTxn) chainID() *big.Int       { return tx.ChainID }
func (tx *MixedTxn) input() []byte           { return tx.Input }
func (tx *MixedTxn) gas() uint64             { return tx.Gas }
func (tx *MixedTxn) gasPrice() *big.Int      { return tx.GasPrice }
func (tx *MixedTxn) gasTipCap() *big.Int     { return tx.GasTipCap }
func (tx *MixedTxn) gasFeeCap() *big.Int     { return tx.GasFeeCap }
func (tx *MixedTxn) value() *big.Int         { return tx.Value }
func (tx *MixedTxn) nonce() uint64           { return tx.Nonce }
func (tx *MixedTxn) to() *Address            { return tx.To }
func (tx *MixedTxn) from() Address           { return tx.From }

func (tx *MixedTxn) hash() Hash { return tx.Hash }

func (tx *MixedTxn) rawSignatureValues() (v, r, s *big.Int) {
	return tx.V, tx.R, tx.S
}

func (tx *MixedTxn) accessList() TxAccessList {
	if tx.transactionType() == DynamicFeeTx {
		return tx.AccessList
	}

	return nil
}

// set methods for transaction fields
func (tx *MixedTxn) setSignatureValues(v, r, s *big.Int) {
	tx.V, tx.R, tx.S = v, r, s
}

func (tx *MixedTxn) setFrom(addr Address) {
	tx.From = addr
}

func (tx *MixedTxn) setGas(gas uint64) {
	tx.Gas = gas
}

func (tx *MixedTxn) setChainID(id *big.Int) {
	tx.ChainID = id
}

func (tx *MixedTxn) setGasPrice(gas *big.Int) {
	tx.GasPrice = gas
}

func (tx *MixedTxn) setGasFeeCap(gas *big.Int) {
	tx.GasFeeCap = gas
}

func (tx *MixedTxn) setGasTipCap(gas *big.Int) {
	tx.GasTipCap = gas
}

func (tx *MixedTxn) setTransactionType(t TxType) {
	tx.Type = t
}

func (tx *MixedTxn) setValue(value *big.Int) {
	tx.Value = value
}

func (tx *MixedTxn) setInput(input []byte) {
	tx.Input = input
}

func (tx *MixedTxn) setTo(addeess *Address) {
	tx.To = addeess
}

func (tx *MixedTxn) setNonce(nonce uint64) {
	tx.Nonce = nonce
}

func (tx *MixedTxn) setAccessList(accessList TxAccessList) {
	tx.AccessList = accessList
}

func (tx *MixedTxn) setHash(h Hash) {
	tx.Hash = h
}
