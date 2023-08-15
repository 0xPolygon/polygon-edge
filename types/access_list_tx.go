package types

import "math/big"

type TxAccessList []AccessTuple

type AccessTuple struct {
	Address     Address
	StorageKeys []Hash
}

// StorageKeys returns the total number of storage keys in the access list.
func (al TxAccessList) StorageKeys() int {
	sum := 0
	for _, tuple := range al {
		sum += len(tuple.StorageKeys)
	}

	return sum
}

// transaction structure
type AccessListStruct struct {
	Nonce    uint64
	GasPrice *big.Int
	Gas      uint64
	To       *Address
	Value    *big.Int
	Input    []byte
	V, R, S  *big.Int
	Hash     Hash
	From     Address

	//Type TxType

	ChainID    *big.Int
	AccessList TxAccessList
}

func (tx *AccessListStruct) transactionType() TxType { return AccessListTx }
func (tx *AccessListStruct) chainID() *big.Int       { return tx.ChainID }
func (tx *AccessListStruct) input() []byte           { return tx.Input }
func (tx *AccessListStruct) gas() uint64             { return tx.Gas }
func (tx *AccessListStruct) gasPrice() *big.Int      { return tx.GasPrice } //check:EIP2718
func (tx *AccessListStruct) gasTipCap() *big.Int     { return tx.GasPrice }
func (tx *AccessListStruct) gasFeeCap() *big.Int     { return tx.GasPrice }
func (tx *AccessListStruct) value() *big.Int         { return tx.Value }
func (tx *AccessListStruct) nonce() uint64           { return tx.Nonce }
func (tx *AccessListStruct) to() *Address            { return tx.To }
func (tx *AccessListStruct) from() Address           { return tx.From }

func (tx *AccessListStruct) hash() Hash { return tx.Hash }

func (tx *AccessListStruct) rawSignatureValues() (v, r, s *big.Int) {
	return tx.V, tx.R, tx.S
}

func (tx *AccessListStruct) accessList() TxAccessList {
	return tx.AccessList
}

// set methods for transaction fields
func (tx *AccessListStruct) setSignatureValues(v, r, s *big.Int) {
	tx.V, tx.R, tx.S = v, r, s
}

func (tx *AccessListStruct) setFrom(addr Address) {
	tx.From = addr
}

func (tx *AccessListStruct) setGas(gas uint64) {
	tx.Gas = gas
}

func (tx *AccessListStruct) setChainID(id *big.Int) {
	tx.ChainID = id
}

func (tx *AccessListStruct) setGasPrice(gas *big.Int) {
	tx.GasPrice = gas
}

func (tx *AccessListStruct) setGasFeeCap(gas *big.Int) {
	tx.GasPrice = gas
}

func (tx *AccessListStruct) setGasTipCap(gas *big.Int) {
	tx.GasPrice = gas
}

func (tx *AccessListStruct) setTransactionType(t TxType) {
	// no need to set a transaction type for access list type of transaction
}

func (tx *AccessListStruct) setValue(value *big.Int) {
	tx.Value = value
}

func (tx *AccessListStruct) setInput(input []byte) {
	tx.Input = input
}

func (tx *AccessListStruct) setTo(addeess *Address) {
	tx.To = addeess
}

func (tx *AccessListStruct) setNonce(nonce uint64) {
	tx.Nonce = nonce
}

func (tx *AccessListStruct) setAccessList(accessList TxAccessList) {
	tx.AccessList = accessList
}

func (tx *AccessListStruct) setHash(h Hash) {
	tx.Hash = h
}
