package types

import (
	"fmt"
	"math/big"
	"sync/atomic"

	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/helper/keccak"
	"github.com/umbracle/fastrlp"
)

const (
	// StateTransactionGasLimit is arbitrary default gas limit for state transactions
	StateTransactionGasLimit = 1000000
)

// TxType is the transaction type.
type TxType byte

// List of supported transaction types
const (
	LegacyTxType     TxType = 0x0
	AccessListTxType TxType = 0x01
	DynamicFeeTxType TxType = 0x02
	StateTxType      TxType = 0x7f
)

func txTypeFromByte(b byte) (TxType, error) {
	tt := TxType(b)

	switch tt {
	case LegacyTxType, StateTxType, DynamicFeeTxType, AccessListTxType:
		return tt, nil
	default:
		return tt, fmt.Errorf("unknown transaction type: %d", b)
	}
}

// String returns string representation of the transaction type.
func (t TxType) String() (s string) {
	switch t {
	case LegacyTxType:
		return "LegacyTx"
	case StateTxType:
		return "StateTx"
	case DynamicFeeTxType:
		return "DynamicFeeTx"
	case AccessListTxType:
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
	t.Inner = inner

	return t
}

// InitInnerData initializes the inner data of a Transaction based on the given transaction type.
// It sets the Inner field of the Transaction to either an AccessListStruct or a MixedTx,
// depending on the value of txType.
func (t *Transaction) InitInnerData(txType TxType) {
	switch txType {
	case AccessListTxType:
		t.Inner = &AccessListTxn{}
	case StateTxType:
		t.Inner = &StateTx{}
	case LegacyTxType:
		t.Inner = &LegacyTx{}
	default:
		t.Inner = &DynamicFeeTx{}
	}
}

type TxData interface {
	transactionType() TxType
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
	setValue(*big.Int)
	setInput([]byte)
	setTo(address *Address)
	setNonce(uint64)
	setAccessList(TxAccessList)
	setHash(Hash)
	unmarshalRLPFrom(p *fastrlp.Parser, v *fastrlp.Value) error
	marshalRLPWith(arena *fastrlp.Arena) *fastrlp.Value
	copy() TxData
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

func (t *Transaction) MarshalRLPWith(a *fastrlp.Arena) *fastrlp.Value {
	return t.Inner.marshalRLPWith(a)
}

func (t *Transaction) UnmarshalRLPFrom(p *fastrlp.Parser, v *fastrlp.Value) error {
	return t.Inner.unmarshalRLPFrom(p, v)
}

// IsContractCreation checks if tx is contract creation
func (t *Transaction) IsContractCreation() bool {
	return t.To() == nil
}

// IsValueTransfer checks if tx is a value transfer
func (t *Transaction) IsValueTransfer() bool {
	return t.Value() != nil &&
		t.Value().Sign() > 0 &&
		len(t.Input()) == 0 &&
		!t.IsContractCreation()
}

// ComputeHash computes the hash of the transaction
func (t *Transaction) ComputeHash() *Transaction {
	var txHash Hash

	hash := keccak.DefaultKeccakPool.Get()
	hash.WriteFn(txHash[:0], t.MarshalRLPTo)
	t.SetHash(txHash)
	keccak.DefaultKeccakPool.Put(hash)

	return t
}

func (t *Transaction) Copy() *Transaction {
	if t == nil {
		return nil
	}

	newTx := new(Transaction)
	innerCopy := t.Inner.copy()
	newTx.Inner = innerCopy

	return newTx
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
//   - or calculate a value with formula: min(gasFeeCap, gasTipCap + baseFee);
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
	case DynamicFeeTxType:
		return t.GasTipCap()
	default:
		return t.GasPrice()
	}
}

// GetGasFeeCap gets gas fee cap depending on tx type
// Spec: https://eips.ethereum.org/EIPS/eip-1559#specification
func (t *Transaction) GetGasFeeCap() *big.Int {
	switch t.Type() {
	case DynamicFeeTxType:
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

func NewTxWithType(txType TxType) *Transaction {
	tx := &Transaction{}

	tx.InitInnerData(txType)

	return tx
}
