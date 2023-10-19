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

	ChainID *big.Int

	// Cache
	size atomic.Pointer[uint64]
}

// IsContractCreation checks if tx is contract creation
func (t *Transaction) IsContractCreation() bool {
	return t.To == nil
}

// IsValueTransfer checks if tx is a value transfer
func (t *Transaction) IsValueTransfer() bool {
	return t.Value != nil &&
		t.Value.Sign() > 0 &&
		len(t.Input) == 0 &&
		!t.IsContractCreation()
}

// ComputeHash computes the hash of the transaction
func (t *Transaction) ComputeHash(blockNumber uint64) *Transaction {
	GetTransactionHashHandler(blockNumber).ComputeHash(t)

	return t
}

func (t *Transaction) Copy() *Transaction {
	tt := new(Transaction)
	tt.Nonce = t.Nonce
	tt.From = t.From
	tt.Gas = t.Gas
	tt.Type = t.Type
	tt.Hash = t.Hash

	if t.To != nil {
		newAddress := *t.To
		tt.To = &newAddress
	}

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

	if t.V != nil {
		tt.V = new(big.Int).Set(t.V)
	}

	if t.R != nil {
		tt.R = new(big.Int).Set(t.R)
	}

	if t.S != nil {
		tt.S = new(big.Int).Set(t.S)
	}

	if t.ChainID != nil {
		tt.ChainID = new(big.Int).Set(t.ChainID)
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
//   - or calculate a value with formula: min(gasFeeCap, gasTipCap + baseFee);
func (t *Transaction) GetGasPrice(baseFee uint64) *big.Int {
	if t.GasPrice != nil && t.GasPrice.BitLen() > 0 {
		return new(big.Int).Set(t.GasPrice)
	} else if baseFee == 0 {
		return big.NewInt(0)
	}

	gasFeeCap := new(big.Int)
	if t.GasFeeCap != nil {
		gasFeeCap = gasFeeCap.Set(t.GasFeeCap)
	}

	gasTipCap := new(big.Int)
	if t.GasTipCap != nil {
		gasTipCap = gasTipCap.Set(t.GasTipCap)
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
	switch t.Type {
	case DynamicFeeTx:
		return t.GasTipCap
	default:
		return t.GasPrice
	}
}

// GetGasFeeCap gets gas fee cap depending on tx type
// Spec: https://eips.ethereum.org/EIPS/eip-1559#specification
func (t *Transaction) GetGasFeeCap() *big.Int {
	switch t.Type {
	case DynamicFeeTx:
		return t.GasFeeCap
	default:
		return t.GasPrice
	}
}

// FindTxByHash returns transaction and its index from a slice of transactions
func FindTxByHash(txs []*Transaction, hash Hash) (*Transaction, int) {
	for idx, txn := range txs {
		if txn.Hash == hash {
			return txn, idx
		}
	}

	return nil, -1
}
