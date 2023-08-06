package types

import "math/big"

type MixedTx struct {
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

func (tx *MixedTx) transactionType() TxType { return tx.Type }
func (tx *MixedTx) chainID() *big.Int       { return tx.ChainID }
func (tx *MixedTx) input() []byte           { return tx.Input }
func (tx *MixedTx) gas() uint64             { return tx.Gas }
func (tx *MixedTx) gasPrice() *big.Int      { return tx.GasPrice } //check:EIP2718
func (tx *MixedTx) gasTipCap() *big.Int     { return tx.GasTipCap }
func (tx *MixedTx) gasFeeCap() *big.Int     { return tx.GasFeeCap }
func (tx *MixedTx) value() *big.Int         { return tx.Value }
func (tx *MixedTx) nonce() uint64           { return tx.Nonce }
func (tx *MixedTx) to() *Address            { return tx.To }
func (tx *MixedTx) from() Address           { return tx.From }

func (tx *MixedTx) hash() Hash { return tx.Hash }

func (tx *MixedTx) rawSignatureValues() (v, r, s *big.Int) {
	return tx.V, tx.R, tx.S
}

func (tx *MixedTx) accessList() TxAccessList {
	if tx.transactionType() == DynamicFeeTx {
		return tx.AccessList
	}

	return nil
}

// set methods for transaction fields
func (tx *MixedTx) setSignatureValues(v, r, s *big.Int) {
	tx.V, tx.R, tx.S = v, r, s
}

func (tx *MixedTx) setFrom(addr Address) {
	tx.From = addr
}

func (tx *MixedTx) setGas(gas uint64) {
	tx.Gas = gas
}

func (tx *MixedTx) setChainID(id *big.Int) {
	tx.ChainID = id
}

func (tx *MixedTx) setGasPrice(gas *big.Int) {
	tx.GasPrice = gas
}

func (tx *MixedTx) setGasFeeCap(gas *big.Int) {
	tx.GasFeeCap = gas
}

func (tx *MixedTx) setGasTipCap(gas *big.Int) {
	tx.GasTipCap = gas
}

func (tx *MixedTx) setTransactionType(t TxType) {
	tx.Type = t
}

func (tx *MixedTx) setValue(value *big.Int) {
	tx.Value = value
}

func (tx *MixedTx) setInput(input []byte) {
	tx.Input = input
}

func (tx *MixedTx) setTo(addeess *Address) {
	tx.To = addeess
}

func (tx *MixedTx) setNonce(nonce uint64) {
	tx.Nonce = nonce
}

func (tx *MixedTx) setAccessList(accessList TxAccessList) {
	tx.AccessList = accessList
}

func (tx *MixedTx) setHash(h Hash) {
	tx.Hash = h
}
