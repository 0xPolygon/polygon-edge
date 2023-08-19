package types

import (
	"fmt"
	"math/big"
	"sync/atomic"

	"github.com/0xPolygon/polygon-edge/helper/common"
)

const (
	// StateTransactionGasLimit is arbitrary default gas limit for state transactions
	StateTransactionGasLimit = 1000000
)

// TxType is the transaction type.
type TxType byte

// List of supported transaction types
const (
	LegacyTx     TxType = 0x0
	StateTx      TxType = 0x7f
	DynamicFeeTx TxType = 0x02
	AccessListTx TxType = 0x01
)

func txTypeFromByte(b byte) (TxType, error) {
	tt := TxType(b)

	switch tt {
	case LegacyTx, StateTx, DynamicFeeTx, AccessListTx:
		return tt, nil
	default:
		return tt, fmt.Errorf("unknown transaction type: %d", b)
	}
}

// String returns string representation of the transaction type.
func (t TxType) String() (s string) {
	switch t {
	case LegacyTx:
		return "LegacyTx"
	case StateTx:
		return "StateTx"
	case DynamicFeeTx:
		return "DynamicFeeTx"
	case AccessListTx:
		return "AccessListTx"
	}

	return
}

type Transaction struct {
	Inner TxData

	// Cache
	size atomic.Pointer[uint64]
}

// NewTx creates a new transaction.
func NewTx(inner TxData) *Transaction {
	t := new(Transaction)
	//check EIP2930: diff in geth with deep copy
	t.Inner = inner

	return t
}

type TxData interface {
	transactionType() TxType
	//copy() TxData // creates a deep copy and initializes all fields  //check:Logic Not implemented

	chainID() *big.Int
	nonce() uint64
	gasPrice() *big.Int
	gasTipCap() *big.Int
	gasFeeCap() *big.Int
	gas() uint64
	to() *Address
	value() *big.Int
	input() []byte
	accessList() TxAccessList
	from() Address
	hash() Hash
	rawSignatureValues() (v, r, s *big.Int)

	//methods to set transactions fields
	setSignatureValues(v, r, s *big.Int)
	setFrom(Address)
	setGas(uint64)
	setChainID(*big.Int)
	setGasPrice(*big.Int)
	setGasFeeCap(*big.Int)
	setGasTipCap(*big.Int)
	setTransactionType(TxType)
	setValue(*big.Int)
	setInput([]byte)
	setTo(address *Address)
	setNonce(uint64)
	setAccessList(TxAccessList)
	setHash(Hash)
}

func (t *Transaction) Type() TxType {
	return t.Inner.transactionType()
}

func (t *Transaction) ChainID() *big.Int {
	return t.Inner.chainID()
}

func (t *Transaction) Nonce() uint64 {
	return t.Inner.nonce()
}

func (t *Transaction) GasPrice() *big.Int {
	return t.Inner.gasPrice()
}

func (t *Transaction) GasTipCap() *big.Int {
	return t.Inner.gasTipCap()
}

func (t *Transaction) GasFeeCap() *big.Int {
	return t.Inner.gasFeeCap()
}

func (t *Transaction) Gas() uint64 {
	return t.Inner.gas()
}

func (t *Transaction) To() *Address {
	return t.Inner.to()
}

func (t *Transaction) Value() *big.Int {
	return t.Inner.value()
}

func (t *Transaction) Input() []byte {
	return t.Inner.input()
}

func (t *Transaction) AccessList() TxAccessList {
	return t.Inner.accessList()
}

func (t *Transaction) From() Address {
	return t.Inner.from()
}

func (t *Transaction) Hash() Hash {
	return t.Inner.hash()
}

func (t *Transaction) RawSignatureValues() (v, r, s *big.Int) {
	return t.Inner.rawSignatureValues()
}

// set methods for transaction fields
func (t *Transaction) SetSignatureValues(v, r, s *big.Int) {
	t.Inner.setSignatureValues(v, r, s)
}

func (t *Transaction) SetFrom(addr Address) {
	t.Inner.setFrom(addr)
}

func (t *Transaction) SetGas(gas uint64) {
	t.Inner.setGas(gas)
}

func (t *Transaction) SetChainID(id *big.Int) {
	t.Inner.setChainID(id)
}

func (t *Transaction) SetGasPrice(gas *big.Int) {
	t.Inner.setGasPrice(gas)
}

func (t *Transaction) SetGasFeeCap(gas *big.Int) {
	t.Inner.setGasFeeCap(gas)
}

func (t *Transaction) SetGasTipCap(gas *big.Int) {
	t.Inner.setGasTipCap(gas)
}

func (t *Transaction) SetTransactionType(tType TxType) {
	t.Inner.setTransactionType(tType)
}

func (t *Transaction) SetValue(value *big.Int) {
	t.Inner.setValue(value)
}

func (t *Transaction) SetInput(input []byte) {
	t.Inner.setInput(input)
}

func (t *Transaction) SetTo(address *Address) {
	t.Inner.setTo(address)
}

func (t *Transaction) SetNonce(nonce uint64) {
	t.Inner.setNonce(nonce)
}

func (t *Transaction) SetAccessList(accessList TxAccessList) {
	t.Inner.setAccessList(accessList)
}

func (t *Transaction) SetHash(h Hash) {
	t.Inner.setHash(h)
}

// IsContractCreation checks if tx is contract creation
func (t *Transaction) IsContractCreation() bool {
	return t.To() == nil
}

// ComputeHash computes the hash of the transaction
func (t *Transaction) ComputeHash(blockNumber uint64) *Transaction {
	GetTransactionHashHandler(blockNumber).ComputeHash(t)

	return t
}

func (t *Transaction) Copy() *Transaction {
	if t == nil {
		return nil
	}

	newTx := new(Transaction)
	innerCopy := DeepCopyTxData(t.Inner)
	newTx.Inner = innerCopy

	return newTx
}

func DeepCopyTxData(data TxData) TxData {
	if data == nil {
		return nil
	}

	switch t := data.(type) {
	case *MixedTx:
		newMixedTx := &MixedTx{}
		*newMixedTx = *t

		newMixedTx.GasPrice = new(big.Int)
		if t.GasPrice != nil {
			newMixedTx.GasPrice.Set(t.GasPrice)
		}

		newMixedTx.GasTipCap = new(big.Int)
		if t.GasTipCap != nil {
			newMixedTx.GasTipCap.Set(t.GasTipCap)
		}

		newMixedTx.GasFeeCap = new(big.Int)
		if t.GasFeeCap != nil {
			newMixedTx.GasFeeCap.Set(t.GasFeeCap)
		}

		newMixedTx.Value = new(big.Int)
		if t.Value != nil {
			newMixedTx.Value.Set(t.Value)
		}

		if newMixedTx.V != nil {
			newMixedTx.V = new(big.Int).Set(t.V)
		}

		if newMixedTx.R != nil {
			newMixedTx.R = new(big.Int).Set(t.R)
		}

		if newMixedTx.S != nil {
			newMixedTx.S = new(big.Int).Set(t.S)
		}

		newMixedTx.Input = make([]byte, len(t.Input))

		copy(newMixedTx.Input[:], t.Input[:])

		//deep copy access list
		newMixedTx.AccessList = DeepCopyTxAccessList(t.AccessList)

		return newMixedTx

	case *AccessListStruct:
		newAccessListStruct := &AccessListStruct{}
		*newAccessListStruct = *t

		newAccessListStruct.GasPrice = new(big.Int)
		if t.GasPrice != nil {
			newAccessListStruct.GasPrice.Set(t.GasPrice)
		}

		newAccessListStruct.Value = new(big.Int)
		if t.Value != nil {
			newAccessListStruct.Value.Set(t.Value)
		}

		if newAccessListStruct.V != nil {
			newAccessListStruct.V = new(big.Int).Set(t.V)
		}

		if newAccessListStruct.R != nil {
			newAccessListStruct.R = new(big.Int).Set(t.R)
		}

		if newAccessListStruct.S != nil {
			newAccessListStruct.S = new(big.Int).Set(t.S)
		}

		newAccessListStruct.Input = make([]byte, len(t.Input))
		copy(newAccessListStruct.Input[:], t.Input[:])

		//deep copy access list
		newAccessListStruct.AccessList = DeepCopyTxAccessList(t.AccessList)

		return newAccessListStruct

	default:
		return nil
	}
}

func DeepCopyTxAccessList(accessList TxAccessList) TxAccessList {
	if accessList == nil {
		return nil
	}

	accessListCopy := make(TxAccessList, len(accessList))

	for i, item := range accessList {
		var copiedAddress Address

		copy(copiedAddress[:], item.Address[:])
		accessListCopy[i] = AccessTuple{
			Address:     copiedAddress,
			StorageKeys: append([]Hash{}, item.StorageKeys...),
		}
	}

	return accessListCopy
}

// Cost returns gas * gasPrice + value
func (t *Transaction) Cost() *big.Int {
	var factor *big.Int

	if t.GasFeeCap() != nil && t.GasFeeCap().BitLen() > 0 {
		factor = new(big.Int).Set(t.GasFeeCap())
	} else {
		factor = new(big.Int).Set(t.GasPrice())
	}

	total := new(big.Int).Mul(factor, new(big.Int).SetUint64(t.Gas()))
	total = total.Add(total, t.Value())

	return total
}

// GetGasPrice returns gas price if not empty, or calculates one based on
// the given EIP-1559 fields if exist
//
// Here is the logic:
//   - use existing gas price if exists
//   - or calculate a value with formula: min(gasFeeCap, gasTipCap * baseFee);
func (t *Transaction) GetGasPrice(baseFee uint64) *big.Int {
	if t.GasPrice() != nil && t.GasPrice().BitLen() > 0 {
		return new(big.Int).Set(t.GasPrice())
	} else if baseFee == 0 {
		return big.NewInt(0)
	}

	gasFeeCap := new(big.Int)
	if t.GasFeeCap() != nil {
		gasFeeCap = gasFeeCap.Set(t.GasFeeCap())
	}

	gasTipCap := new(big.Int)
	if t.GasTipCap() != nil {
		gasTipCap = gasTipCap.Set(t.GasTipCap())
	}

	if gasFeeCap.BitLen() > 0 || gasTipCap.BitLen() > 0 {
		return common.BigMin(
			gasTipCap.Add(
				gasTipCap,
				new(big.Int).SetUint64(baseFee),
			),
			gasFeeCap,
		)
	}

	return big.NewInt(0)
}

func (t *Transaction) Size() uint64 {
	if size := t.size.Load(); size != nil {
		return *size
	}

	size := uint64(len(t.MarshalRLP()))
	t.size.Store(&size)

	return size
}

// EffectiveGasTip defines effective tip based on tx type.
// Spec: https://eips.ethereum.org/EIPS/eip-1559#specification
// We use EIP-1559 fields of the tx if the london hardfork is enabled.
// Effective tip be came to be either gas tip cap or (gas fee cap - current base fee)
func (t *Transaction) EffectiveGasTip(baseFee *big.Int) *big.Int {
	if baseFee == nil || baseFee.BitLen() == 0 {
		return t.GetGasTipCap()
	}

	return common.BigMin(
		new(big.Int).Set(t.GetGasTipCap()),
		new(big.Int).Sub(t.GetGasFeeCap(), baseFee))
}

// GetGasTipCap gets gas tip cap depending on tx type
// Spec: https://eips.ethereum.org/EIPS/eip-1559#specification
func (t *Transaction) GetGasTipCap() *big.Int {
	switch t.Type() {
	case DynamicFeeTx:
		return t.GasTipCap()
	default:
		return t.GasPrice()
	}
}

// GetGasFeeCap gets gas fee cap depending on tx type
// Spec: https://eips.ethereum.org/EIPS/eip-1559#specification
func (t *Transaction) GetGasFeeCap() *big.Int {
	switch t.Type() {
	case DynamicFeeTx:
		return t.GasFeeCap()
	default:
		return t.GasPrice()
	}
}

// FindTxByHash returns transaction and its index from a slice of transactions
func FindTxByHash(txs []*Transaction, hash Hash) (*Transaction, int) {
	for idx, txn := range txs {
		if txn.Hash() == hash {
			return txn, idx
		}
	}

	return nil, -1
}
