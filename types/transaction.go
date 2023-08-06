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
)

func txTypeFromByte(b byte) (TxType, error) {
	tt := TxType(b)

	switch tt {
	case LegacyTx, StateTx, DynamicFeeTx:
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
	tx := new(Transaction)
	//check: diff in geth with deep copy
	tx.Inner = inner
	return tx
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

func (tx *Transaction) Type() TxType {
	return tx.Inner.transactionType()
}

func (tx *Transaction) ChainID() *big.Int {
	return tx.Inner.chainID()
}

func (tx *Transaction) Nonce() uint64 {
	return tx.Inner.nonce()
}

func (tx *Transaction) GasPrice() *big.Int {
	return tx.Inner.gasPrice()
}

func (tx *Transaction) GasTipCap() *big.Int {
	return tx.Inner.gasTipCap()
}

func (tx *Transaction) GasFeeCap() *big.Int {
	return tx.Inner.gasFeeCap()
}

func (tx *Transaction) Gas() uint64 {
	return tx.Inner.gas()
}

func (tx *Transaction) To() *Address {
	return tx.Inner.to()
}

func (tx *Transaction) Value() *big.Int {
	return tx.Inner.value()
}

func (tx *Transaction) Input() []byte {
	return tx.Inner.input()
}

func (tx *Transaction) AccessList() TxAccessList {
	return tx.Inner.accessList()
}

func (tx *Transaction) From() Address {
	return tx.Inner.from()
}

func (tx *Transaction) Hash() Hash {
	return tx.Inner.hash()
}

func (tx *Transaction) RawSignatureValues() (v, r, s *big.Int) {
	return tx.Inner.rawSignatureValues()
}

// set methods for transaction fields
func (tx *Transaction) SetSignatureValues(v, r, s *big.Int) {
	tx.Inner.setSignatureValues(v, r, s)
}

func (tx *Transaction) SetFrom(addr Address) {
	tx.Inner.setFrom(addr)
}

func (tx *Transaction) SetGas(gas uint64) {
	tx.Inner.setGas(gas)
}

func (tx *Transaction) SetChainID(id *big.Int) {
	tx.Inner.setChainID(id)
}

func (tx *Transaction) SetGasPrice(gas *big.Int) {
	tx.Inner.setGasPrice(gas)
}

func (tx *Transaction) SetGasFeeCap(gas *big.Int) {
	tx.Inner.setGasFeeCap(gas)
}

func (tx *Transaction) SetGasTipCap(gas *big.Int) {
	tx.Inner.setGasTipCap(gas)
}

func (tx *Transaction) SetTransactionType(t TxType) {
	tx.Inner.setTransactionType(t)
}

func (tx *Transaction) SetValue(value *big.Int) {
	tx.Inner.setValue(value)
}

func (tx *Transaction) SetInput(input []byte) {
	tx.Inner.setInput(input)
}

func (tx *Transaction) SetTo(address *Address) {
	tx.Inner.setTo(address)
}

func (tx *Transaction) SetNonce(nonce uint64) {
	tx.Inner.setNonce(nonce)
}

func (tx *Transaction) SetAccessList(accessList TxAccessList) {
	tx.Inner.setAccessList(accessList)
}

func (tx *Transaction) SetHash(h Hash) {
	tx.Inner.setHash(h)
}

// type Transaction struct {
// 	Nonce     uint64
// 	GasPrice  *big.Int
// 	GasTipCap *big.Int
// 	GasFeeCap *big.Int
// 	Gas       uint64
// 	To        *Address
// 	Value     *big.Int
// 	Input     []byte
// 	V, R, S   *big.Int
// 	Hash      Hash
// 	From      Address

// 	Type TxType

// 	ChainID *big.Int

// 	// Cache
// 	size atomic.Pointer[uint64]

// 	AccessList TxAccessList
// }

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

// func DeepCopyTxData(data TxData) TxData {
// 	if data == nil {
// 		return nil
// 	}

// 	switch t := data.(type) {
// 	case *MixedTx:
// 		v, r, s := t.rawSignatureValues()
// 		return &MixedTx{
// 			Nonce:      t.nonce(),
// 			GasPrice:   new(big.Int).Set(t.gasPrice()),
// 			GasTipCap:  new(big.Int).Set(t.gasTipCap()),
// 			GasFeeCap:  new(big.Int).Set(t.gasFeeCap()),
// 			Gas:        t.gas(),
// 			To:         t.to(),
// 			Value:      new(big.Int).Set(t.value()),
// 			Input:      append([]byte(nil), t.input()...),
// 			V:          new(big.Int).Set(v),
// 			R:          new(big.Int).Set(r),
// 			S:          new(big.Int).Set(s),
// 			Hash:       t.hash(),
// 			From:       t.from(),
// 			Type:       t.transactionType(),
// 			ChainID:    t.chainID(),
// 			AccessList: DeepCopyTxAccessList(t.accessList()),
// 		}

//		default:
//			return nil
//		}
//	}
func DeepCopyTxData(data TxData) TxData {
	if data == nil {
		return nil
	}

	switch t := data.(type) {
	case *MixedTx:
		// Initialize empty values for fields that may not be present in all TxData implementations
		// var nonce uint64
		// var gasPrice, gasTipCap, gasFeeCap, value, chainID *big.Int
		// var gas uint64
		// var to *Address
		// var input []byte
		// var v, r, s *big.Int
		// var hash Hash
		// var from Address
		// var transactionType TxType
		// var accessList TxAccessList

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

		if newMixedTx.R != nil {
			newMixedTx.R = new(big.Int)
			newMixedTx.R = big.NewInt(0).SetBits(t.R.Bits())
		}

		if newMixedTx.S != nil {
			newMixedTx.S = new(big.Int)
			newMixedTx.S = big.NewInt(0).SetBits(t.S.Bits())
		}

		newMixedTx.Input = make([]byte, len(t.Input))
		copy(newMixedTx.Input[:], t.Input[:])

		return newMixedTx

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

// func (t *Transaction) Copy() *Transaction {
// 	tt := new(Transaction)
// 	*tt = *t

// 	tt.GasPrice = new(big.Int)
// 	if t.GasPrice != nil {
// 		tt.GasPrice.Set(t.GasPrice)
// 	}

// 	tt.GasTipCap = new(big.Int)
// 	if t.GasTipCap != nil {
// 		tt.GasTipCap.Set(t.GasTipCap)
// 	}

// 	tt.GasFeeCap = new(big.Int)
// 	if t.GasFeeCap != nil {
// 		tt.GasFeeCap.Set(t.GasFeeCap)
// 	}

// 	tt.Value = new(big.Int)
// 	if t.Value != nil {
// 		tt.Value.Set(t.Value)
// 	}

// 	if t.R != nil {
// 		tt.R = new(big.Int)
// 		tt.R = big.NewInt(0).SetBits(t.R.Bits())
// 	}

// 	if t.S != nil {
// 		tt.S = new(big.Int)
// 		tt.S = big.NewInt(0).SetBits(t.S.Bits())
// 	}

// 	tt.Input = make([]byte, len(t.Input))
// 	copy(tt.Input[:], t.Input[:])

// 	return tt
// }

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
