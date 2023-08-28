package types

import (
	"fmt"
	"math/big"
	"sync/atomic"

	"github.com/0xPolygon/polygon-edge/helper/common"
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
	size atomic.Pointer[uint64]
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

	if t.GasFeeCap != nil && t.GasFeeCap.BitLen() > 0 {
		factor = new(big.Int).Set(t.GasFeeCap)
	} else {
		factor = new(big.Int).Set(t.GasPrice)
	}

	total := new(big.Int).Mul(factor, new(big.Int).SetUint64(t.Gas))
	total = total.Add(total, t.Value)

	return total
}

// GetGasPrice returns gas price if not empty, or calculates one based on
// the given EIP-1559 fields if exist
//
// Here is the logic:
//   - use existing gas price if exists
//   - or calculate a value with formula: min(gasFeeCap, gasTipCap * baseFee);
func (t *Transaction) GetGasPrice(baseFee uint64) *big.Int {
	if t.GasPrice != nil && t.GasPrice.BitLen() > 0 {
		return new(big.Int).Set(t.GasPrice)
	} else if baseFee == 0 {
		return new(big.Int)
	}

	gasFeeCap := new(big.Int)
	if t.GasFeeCap != nil {
		gasFeeCap = gasFeeCap.Set(t.GasFeeCap)
	}

	gasTipCap := new(big.Int)
	if t.GasTipCap != nil {
		gasTipCap = gasTipCap.Set(t.GasTipCap)
	}

	gasPrice := new(big.Int)
	if gasFeeCap.BitLen() > 0 || gasTipCap.BitLen() > 0 {
		gasPrice = common.BigMin(
			new(big.Int).Add(
				gasTipCap,
				new(big.Int).SetUint64(baseFee),
			),
			new(big.Int).Set(gasFeeCap),
		)
	}

	return gasPrice
}

func (t *Transaction) Size() uint64 {
	if size := t.size.Load(); size != nil {
		return *size
	}

	size := uint64(len(t.MarshalRLP()))
	t.size.Store(&size)

	return size
}

// EffectiveTip defines effective tip based on tx type.
// Spec: https://eips.ethereum.org/EIPS/eip-1559#specification
// We use EIP-1559 fields of the tx if the london hardfork is enabled.
// Effective tip be came to be either gas tip cap or (gas fee cap - current base fee)
func (t *Transaction) EffectiveTip(baseFee uint64) *big.Int {
	if t.GasFeeCap != nil && t.GasTipCap != nil {
		return common.BigMin(
			new(big.Int).Sub(t.GasFeeCap, new(big.Int).SetUint64(baseFee)),
			new(big.Int).Set(t.GasTipCap),
		)
	}

	return t.GetGasPrice(baseFee)
}
