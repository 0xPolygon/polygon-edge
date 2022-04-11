package types

import (
	"fmt"
	"math/big"
	"sync/atomic"

	"github.com/0xPolygon/polygon-edge/helper/keccak"
)

// TransactionType represents the value between 0x00 and 0x7F to represents the type of transaction
// TransactionType is put before transaction data in RLP encoded
type TransactionType uint8

const (
	// TxTypeAccessList and TxTypeDynamicFee have been added in Ethereum, but Edge doesn't support at the moment
	// Keep both types for future work

	TxTypeLegacy TransactionType = 0x0
	// TxTypeAccessList TransactionType = 0x1 // EIP-2930
	// TxTypeDynamicFee TransactionType = 0x2 // EIP-1559
	TxTypeState TransactionType = 0x7f
)

func ToTransactionType(b byte) (TransactionType, error) {
	tt := TransactionType(b)
	switch tt {
	case TxTypeLegacy, TxTypeState:
		return tt, nil
	default:
		return TransactionType(0), fmt.Errorf("undefined transaction type: %d", b)
	}
}

func (t TransactionType) String() string {
	switch t {
	case TxTypeLegacy:
		return "TxTypeLegacy"
	case TxTypeState:
		return "TxTypeState"
	default:
		return fmt.Sprintf("TxType(%x)", byte(t))
	}
}

type Transaction struct {
	Nonce    uint64
	GasPrice *big.Int
	Gas      uint64
	To       *Address
	Value    *big.Int
	Input    []byte
	V        *big.Int
	R        *big.Int
	S        *big.Int
	Hash     Hash
	From     Address

	Type TransactionType

	// For StateTransaction
	StateSignatures [][]byte

	// Cache
	size atomic.Value
}

func (t *Transaction) IsContractCreation() bool {
	return t.To == nil
}

func (t *Transaction) IsTypedTransaction() bool {
	return t.Type != TxTypeLegacy
}

// ComputeHash computes the hash of the transaction
func (t *Transaction) ComputeHash() *Transaction {
	hash := keccak.DefaultKeccakPool.Get()
	if _, err := hash.Write(t.MarshalRLP()); err == nil {
		hash.Sum(t.Hash[:0])
		keccak.DefaultKeccakPool.Put(hash)
	}

	return t
}

func (t *Transaction) Copy() *Transaction {
	tt := new(Transaction)
	*tt = *t

	tt.GasPrice = new(big.Int)
	if t.GasPrice != nil {
		tt.GasPrice.Set(t.GasPrice)
	}

	tt.Value = new(big.Int)
	if t.Value != nil {
		tt.Value.Set(t.Value)
	}

	if t.R != nil {
		tt.R = new(big.Int)
		tt.R = big.NewInt(0).SetBits(t.R.Bits())
	}

	if t.S != nil {
		tt.S = new(big.Int)
		tt.S = big.NewInt(0).SetBits(t.S.Bits())
	}

	tt.Input = make([]byte, len(t.Input))
	copy(tt.Input[:], t.Input[:])

	if t.StateSignatures != nil {
		tt.StateSignatures = make([][]byte, len(t.StateSignatures))
		for i := range t.StateSignatures {
			tt.StateSignatures[i] = make([]byte, len(t.StateSignatures[i]))
			copy(tt.StateSignatures[i][:], t.StateSignatures[i][:])
		}
	}

	return tt
}

// Cost returns gas * gasPrice + value
func (t *Transaction) Cost() *big.Int {
	total := new(big.Int).Mul(t.GasPrice, new(big.Int).SetUint64(t.Gas))
	total.Add(total, t.Value)

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
	return t.Gas > blockGasLimit
}

func (t *Transaction) IsUnderpriced(priceLimit uint64) bool {
	return t.GasPrice.Cmp(big.NewInt(0).SetUint64(priceLimit)) < 0
}
