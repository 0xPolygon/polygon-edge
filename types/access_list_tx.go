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

// Copy makes a deep copy of the access list.
func (al TxAccessList) Copy() TxAccessList {
	if al == nil {
		return nil
	}

	newAccessList := make(TxAccessList, len(al))

	for i, item := range al {
		var copiedAddress Address

		copy(copiedAddress[:], item.Address[:])
		newAccessList[i] = AccessTuple{
			Address:     copiedAddress,
			StorageKeys: append([]Hash{}, item.StorageKeys...),
		}
	}

	return newAccessList
}

type AccessListTxn struct {
	Nonce    uint64
	GasPrice *big.Int
	Gas      uint64
	To       *Address
	Value    *big.Int
	Input    []byte
	V, R, S  *big.Int
	Hash     Hash
	From     Address

	ChainID    *big.Int
	AccessList TxAccessList
}

func (tx *AccessListTxn) transactionType() TxType { return AccessListTx }
func (tx *AccessListTxn) chainID() *big.Int       { return tx.ChainID }
func (tx *AccessListTxn) input() []byte           { return tx.Input }
func (tx *AccessListTxn) gas() uint64             { return tx.Gas }
func (tx *AccessListTxn) gasPrice() *big.Int      { return tx.GasPrice }
func (tx *AccessListTxn) gasTipCap() *big.Int     { return tx.GasPrice }
func (tx *AccessListTxn) gasFeeCap() *big.Int     { return tx.GasPrice }
func (tx *AccessListTxn) value() *big.Int         { return tx.Value }
func (tx *AccessListTxn) nonce() uint64           { return tx.Nonce }
func (tx *AccessListTxn) to() *Address            { return tx.To }
func (tx *AccessListTxn) from() Address           { return tx.From }

func (tx *AccessListTxn) hash() Hash { return tx.Hash }

func (tx *AccessListTxn) rawSignatureValues() (v, r, s *big.Int) {
	return tx.V, tx.R, tx.S
}

func (tx *AccessListTxn) accessList() TxAccessList {
	return tx.AccessList
}

// set methods for transaction fields
func (tx *AccessListTxn) setSignatureValues(v, r, s *big.Int) {
	tx.V, tx.R, tx.S = v, r, s
}

func (tx *AccessListTxn) setFrom(addr Address) {
	tx.From = addr
}

func (tx *AccessListTxn) setGas(gas uint64) {
	tx.Gas = gas
}

func (tx *AccessListTxn) setChainID(id *big.Int) {
	tx.ChainID = id
}

func (tx *AccessListTxn) setGasPrice(gas *big.Int) {
	tx.GasPrice = gas
}

func (tx *AccessListTxn) setGasFeeCap(gas *big.Int) {
	tx.GasPrice = gas
}

func (tx *AccessListTxn) setGasTipCap(gas *big.Int) {
	tx.GasPrice = gas
}

func (tx *AccessListTxn) setTransactionType(t TxType) {
	// no need to set a transaction type for access list type of transaction
}

func (tx *AccessListTxn) setValue(value *big.Int) {
	tx.Value = value
}

func (tx *AccessListTxn) setInput(input []byte) {
	tx.Input = input
}

func (tx *AccessListTxn) setTo(addeess *Address) {
	tx.To = addeess
}

func (tx *AccessListTxn) setNonce(nonce uint64) {
	tx.Nonce = nonce
}

func (tx *AccessListTxn) setAccessList(accessList TxAccessList) {
	tx.AccessList = accessList
}

func (tx *AccessListTxn) setHash(h Hash) {
	tx.Hash = h
}
