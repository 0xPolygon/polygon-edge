package types

import (
	"fmt"
	"math/big"
	"sync/atomic"
	"time"

	"github.com/0xPolygon/polygon-edge/helper/keccak"
	"github.com/umbracle/fastrlp"
)

type TxType byte

const (
	LegacyTxType TxType = 0x0
	StateTxType  TxType = 0x7f
	DynamicFeeTx TxType = 0x8f

	StateTransactionGasLimit = 1000000 // some arbitrary default gas limit for state transactions
)

func ReadRlpTxType(rlpValue *fastrlp.Value) (TxType, error) {
	bytes, err := rlpValue.Bytes()
	if err != nil {
		return LegacyTxType, err
	}

	if len(bytes) != 1 {
		return LegacyTxType, fmt.Errorf("expected 1 byte transaction type, but size is %d", len(bytes))
	}

	b := TxType(bytes[0])

	switch b {
	case LegacyTxType, StateTxType:
		return b, nil
	default:
		return LegacyTxType, fmt.Errorf("invalid tx type value: %d", bytes[0])
	}
}

func (t TxType) String() (s string) {
	switch t {
	case LegacyTxType:
		return "LegacyTx"
	case StateTxType:
		return "StateTx"
	default:
		return "UnknownTX"
	}
}

type TxData interface {
	txType() TxType // returns the type ID
	Copy() TxData   // creates a deep copy and initializes all fields

	input() []byte
	gas() uint64
	setGas(gas uint64)
	gasPrice() *big.Int
	gasTipCap() *big.Int
	gasFeeCap() *big.Int
	value() *big.Int
	nonce() uint64
	to() *Address

	rawSignatureValues() (v, r, s *big.Int)
	setSignatureValues(v, r, s *big.Int)
}

type Transaction struct {
	inner TxData    // Consensus contents of a transaction
	time  time.Time // Time first seen locally (spam avoidance)

	// Caches
	hash Hash
	size atomic.Value
	from Address
}

// NewTx creates a new transaction.
func NewTx(inner TxData) *Transaction {
	tx := &Transaction{}
	tx.setDecoded(inner.Copy(), 0)

	return tx
}

// NewTxWithSender creates a new transaction with the given sender address.
func NewTxWithSender(inner TxData, sender Address) *Transaction {
	tx := &Transaction{
		from: sender,
	}

	tx.setDecoded(inner.Copy(), 0)

	return tx
}

// IsContractCreation checks if tx is contract creation
func (t *Transaction) IsContractCreation() bool {
	return t.inner.to() == nil
}

func (t *Transaction) IsLegacyTx() bool {
	return t.inner.txType() == LegacyTxType
}

func (t *Transaction) IsStateTx() bool {
	return t.inner.txType() == StateTxType
}

// ComputeHash computes the hash of the transaction
func (t *Transaction) ComputeHash() *Transaction {
	ar := marshalArenaPool.Get()
	hash := keccak.DefaultKeccakPool.Get()

	v := t.MarshalRLPWith(ar)
	hash.WriteRlp(t.hash[:0], v)

	marshalArenaPool.Put(ar)
	keccak.DefaultKeccakPool.Put(hash)

	return t
}

// Copy makes a copy of the given transaction
func (t *Transaction) Copy() *Transaction {
	return &Transaction{
		inner: t.inner.Copy(),
		time:  t.time,
		hash:  t.hash,
		size:  t.size,
		from:  t.from,
	}
}

// Cost returns gas * gasPrice + value
func (t *Transaction) Cost() *big.Int {
	total := new(big.Int).Mul(t.inner.gasPrice(), new(big.Int).SetUint64(t.inner.gas()))
	total.Add(total, t.inner.value())

	return total
}

func (t *Transaction) Size() uint64 {
	if size := t.size.Load(); size != nil {
		sizeVal, ok := size.(uint64)
		if !ok {
			return 0
		}

		return sizeVal
	}

	size := uint64(len(t.MarshalRLP()))
	t.size.Store(size)

	return size
}

func (t *Transaction) ExceedsBlockGasLimit(blockGasLimit uint64) bool {
	return t.inner.gas() > blockGasLimit
}

func (t *Transaction) IsUnderpriced(priceLimit uint64) bool {
	return t.inner.gasPrice().Cmp(big.NewInt(0).SetUint64(priceLimit)) < 0
}

// Type returns the transaction type.
func (t *Transaction) Type() TxType {
	return t.inner.txType()
}

// Gas returns the gas limit of the transaction.
func (t *Transaction) Gas() uint64 { return t.inner.gas() }

// SetGas sets the gas limit of the transaction.
func (t *Transaction) SetGas(gas uint64) { t.inner.setGas(gas) }

// GasPrice returns the gas price of the transaction.
func (t *Transaction) GasPrice() *big.Int { return new(big.Int).Set(t.inner.gasPrice()) }

// GasTipCap returns the gasTipCap per gas of the transaction.
func (t *Transaction) GasTipCap() *big.Int { return new(big.Int).Set(t.inner.gasTipCap()) }

// GasFeeCap returns the fee cap per gas of the transaction.
func (t *Transaction) GasFeeCap() *big.Int { return new(big.Int).Set(t.inner.gasFeeCap()) }

// Value returns the ether amount of the transaction.
func (t *Transaction) Value() *big.Int { return new(big.Int).Set(t.inner.value()) }

// Input returns the input data of the transaction.
func (t *Transaction) Input() []byte { return t.inner.input() }

// Nonce returns the sender account nonce of the transaction.
func (t *Transaction) Nonce() uint64 { return t.inner.nonce() }

// To returns the recipient address of the transaction.
// For contract-creation transactions, To returns nil.
func (t *Transaction) To() *Address {
	return copyAddressPtr(t.inner.to())
}

// From returns the sender address of the transaction
func (t *Transaction) From() Address {
	return t.from
}

// SetSender sets the given the sender address of the transaction
func (t *Transaction) SetSender(sender Address) {
	t.from = sender
}

// RawSignatureValues returns the V, R, S signature values of the transaction.
// The return values should not be modified by the caller.
func (t *Transaction) RawSignatureValues() (v, r, s *big.Int) {
	return t.inner.rawSignatureValues()
}

// Hash returns the transaction hash.
func (t *Transaction) Hash() Hash {
	return t.hash
}

// SetSignatureValues sets the given signature values
func (t *Transaction) SetSignatureValues(v, r, s *big.Int) *Transaction {
	t.inner.setSignatureValues(v, r, s)
	t.ComputeHash()

	return t
}

// setDecoded sets the inner transaction and size after decoding.
func (t *Transaction) setDecoded(inner TxData, size uint64) {
	t.inner = inner
	t.time = time.Now()

	if size > 0 {
		t.size.Store(size)
	}
}

// copyAddressPtr copies an address.
func copyAddressPtr(a *Address) *Address {
	if a == nil {
		return nil
	}

	cpy := *a

	return &cpy
}
