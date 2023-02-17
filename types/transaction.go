package types

import (
	"fmt"
	"math/big"
	"sync/atomic"

	"github.com/0xPolygon/polygon-edge/helper/keccak"
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

	// Cache
	size atomic.Value
}

// IsContractCreation checks if tx is contract creation
func (t *Transaction) IsContractCreation() bool {
	return t.To == nil
}

// ComputeHash computes the hash of the transaction
func (t *Transaction) ComputeHash() *Transaction {
	ar := marshalArenaPool.Get()
	hash := keccak.DefaultKeccakPool.Get()

	v := t.MarshalRLPWith(ar)
	hash.WriteRlp(t.Hash[:0], v)

	marshalArenaPool.Put(ar)
	keccak.DefaultKeccakPool.Put(hash)

	return t
}

func (t *Transaction) Copy() *Transaction {
	tt := new(Transaction)
	*tt = *t

	tt.GasPrice = new(big.Int)
	if t.GasPrice != nil {
		tt.GasPrice.Set(t.GasPrice)
	}

	tt.GasTipCap = new(big.Int)
	if t.GasTipCap != nil {
		tt.GasTipCap.Set(t.GasTipCap)
	}

	tt.GasFeeCap = new(big.Int)
	if t.GasFeeCap != nil {
		tt.GasFeeCap.Set(t.GasFeeCap)
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

	return tt
}

// Cost returns gas * gasPrice + value
func (t *Transaction) Cost() *big.Int {
	var factor *big.Int

	if t.GasFeeCap != nil || t.GasFeeCap.BitLen() > 0 {
		factor = new(big.Int).Set(t.GasFeeCap)
	} else {
		factor = new(big.Int).Set(t.GasPrice)
	}

	total := new(big.Int).Mul(factor, new(big.Int).SetUint64(t.Gas))
	total = total.Add(total, t.Value)

	return total
}

// PrefillFees fills fee-related fields depending on the provided input.
// Basically, there must be either gas price OR gas fee cap and gas tip cap provided.
//
// Here is the logic:
//   - use gas price for gas tip cap and gas fee cap if base fee is nil;
//   - otherwise, if base fee is not provided:
//   - use gas price for gas tip cap and gas fee cap if gas price is not nil;
//   - otherwise, if base tip cap and base fee cap are provided:
//   - gas price should be min(gasFeeCap, gasTipCap * baseFee);
func (t *Transaction) PrefillFees(baseFee uint64) {
	// Do nothing if fees are there
	if t.GasPrice != nil && t.GasTipCap != nil && t.GasFeeCap != nil &&
		t.GasPrice.BitLen() > 0 && t.GasTipCap.BitLen() > 0 && t.GasFeeCap.BitLen() > 0 {
		return
	}

	if baseFee == 0 {
		// If there's no basefee, then it must be a non-1559 execution
		if t.GasPrice == nil {
			t.GasPrice = new(big.Int)
		}

		t.GasFeeCap = new(big.Int).Set(t.GasPrice)
		t.GasTipCap = new(big.Int).Set(t.GasPrice)

		return
	}

	// A basefee is provided, necessitating 1559-type execution
	if t.GasPrice != nil {
		// User specified the legacy gas field, convert to 1559 gas typing
		t.GasFeeCap = new(big.Int).Set(t.GasPrice)
		t.GasTipCap = new(big.Int).Set(t.GasPrice)

		return
	}

	// User specified 1559 gas feilds (or none), use those
	if t.GasFeeCap == nil {
		t.GasFeeCap = new(big.Int)
	}

	if t.GasTipCap == nil {
		t.GasTipCap = new(big.Int)
	}

	// Backfill the legacy gasPrice for EVM execution, unless we're all zeroes
	t.GasPrice = new(big.Int)

	if t.GasFeeCap.BitLen() > 0 || t.GasTipCap.BitLen() > 0 {
		t.GasPrice = new(big.Int).Add(
			t.GasTipCap,
			new(big.Int).SetUint64(baseFee),
		)

		if t.GasPrice.Cmp(t.GasFeeCap) > 0 {
			t.GasPrice = new(big.Int).Set(t.GasFeeCap)
		}
	}
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
