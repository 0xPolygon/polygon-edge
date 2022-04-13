package types

import (
	"fmt"
	"math/big"
	"sync/atomic"

	"github.com/0xPolygon/polygon-edge/helper/keccak"
	"github.com/umbracle/fastrlp"
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

type Transactions []*Transaction

type Transaction struct {
	size    atomic.Value
	hash    Hash
	Payload TxPayload
}

func (t *Transaction) IsTypedTransaction() bool {
	return t.Type() != TxTypeLegacy
}

func (t *Transaction) Type() TransactionType {
	return t.Payload.txType()
}

func (t *Transaction) Nonce() uint64 {
	return t.Payload.nonce()
}

func (t *Transaction) From() Address {
	return t.Payload.from()
}

func (t *Transaction) SetFrom(from Address) {
	t.Payload.setFrom(from)
}

func (t *Transaction) To() *Address {
	return t.Payload.to()
}

func (t *Transaction) Value() *big.Int {
	return t.Payload.value()
}

func (t *Transaction) Input() []byte {
	return t.Payload.input()
}

func (t *Transaction) Gas() uint64 {
	return t.Payload.gas()
}

func (t *Transaction) SetGas(gas uint64) {
	t.Payload.setGas(gas)
}

func (t *Transaction) GasPrice() *big.Int {
	return t.Payload.gasPrice()
}

func (t *Transaction) IsContractCreation() bool {
	return t.Payload.isContractCreation()
}

func (t *Transaction) ExceedsBlockGasLimit(blockGasLimit uint64) bool {
	return t.Payload.exceedsBlockGasLimit(blockGasLimit)
}

// Cost returns gas * gasPrice + value
func (t *Transaction) Cost() *big.Int {
	total := new(big.Int).Mul(t.GasPrice(), new(big.Int).SetUint64(t.Gas()))
	total.Add(total, t.Value())

	return total
}

func (t *Transaction) IsUnderpriced(priceLimit uint64) bool {
	return t.GasPrice().Cmp(big.NewInt(0).SetUint64(priceLimit)) < 0
}

func (t *Transaction) Hash() Hash {
	if t.hash == ZeroHash {
		return t.ComputeHash().hash
	}

	return t.hash
}

func (t *Transaction) ComputeHash() *Transaction {
	hash := keccak.DefaultKeccakPool.Get()

	if _, err := hash.Write(t.MarshalRLP()); err == nil {
		hash.Sum(t.hash[:0])
		keccak.DefaultKeccakPool.Put(hash)
	}

	return t
}

func (t *Transaction) Size() uint64 {
	if size := t.size.Load(); size != nil {
		if sizeVal, ok := size.(uint64); ok {
			return sizeVal
		}
	}

	// Calculate and store size
	size := uint64(len(t.MarshalRLP()))
	t.size.Store(size)

	return size
}

func (t *Transaction) Copy() *Transaction {
	tt := new(Transaction)
	*tt = *t

	// size
	if size := t.size.Load(); size != nil {
		if sizeVal, ok := size.(uint64); ok {
			tt.size.Store(sizeVal)
		}
	}

	// hash
	copy(tt.hash[:], t.hash[:])

	// payload
	tt.Payload = t.Payload.copy()

	return tt
}

type TxPayload interface {
	txType() TransactionType
	nonce() uint64
	from() Address
	setFrom(Address)
	to() *Address
	value() *big.Int
	input() []byte
	gas() uint64
	setGas(uint64)
	gasPrice() *big.Int
	isContractCreation() bool
	exceedsBlockGasLimit(uint64) bool
	copy() TxPayload

	// RLP encode/decode
	MarshalRLPWith(*fastrlp.Arena) *fastrlp.Value
	MarshalStoreRLPWith(a *fastrlp.Arena) *fastrlp.Value
	UnmarshalRLPFrom(p *fastrlp.Parser, v *fastrlp.Value) error
	UnmarshalStoreRLPFrom(p *fastrlp.Parser, v *fastrlp.Value) error
}

type SignatureSetterGetter interface {
	SetSignatureValues(v, r, s *big.Int)
	GetSignatureValues() (v, r, s *big.Int)
}

func newTxPayload(txType TransactionType) (TxPayload, error) {
	switch txType {
	case TxTypeLegacy:
		return &LegacyTransaction{}, nil
	case TxTypeState:
		return &StateTransaction{}, nil
	}

	return nil, fmt.Errorf("undefined transaction type: %d", txType)
}

type LegacyTransaction struct {
	Nonce    uint64
	GasPrice *big.Int
	Gas      uint64
	To       *Address
	Value    *big.Int
	Input    []byte
	V        *big.Int
	R        *big.Int
	S        *big.Int

	From Address
}

func (t *LegacyTransaction) txType() TransactionType {
	return TxTypeLegacy
}

func (t *LegacyTransaction) nonce() uint64 {
	return t.Nonce
}

func (t *LegacyTransaction) from() Address {
	return t.From
}

func (t *LegacyTransaction) setFrom(from Address) {
	t.From = from
}

func (t *LegacyTransaction) to() *Address {
	return t.To
}

func (t *LegacyTransaction) value() *big.Int {
	return t.Value
}

func (t *LegacyTransaction) input() []byte {
	return t.Input
}

func (t *LegacyTransaction) gas() uint64 {
	return t.Gas
}

func (t *LegacyTransaction) setGas(gas uint64) {
	t.Gas = gas
}

func (t *LegacyTransaction) gasPrice() *big.Int {
	return t.GasPrice
}

func (t *LegacyTransaction) isContractCreation() bool {
	return t.To == nil
}

func (t *LegacyTransaction) exceedsBlockGasLimit(blockGasLimit uint64) bool {
	return t.Gas > blockGasLimit
}

func (t *LegacyTransaction) copy() TxPayload {
	tt := new(LegacyTransaction)
	*tt = *t

	tt.GasPrice = new(big.Int)
	if t.GasPrice != nil {
		tt.GasPrice.Set(t.GasPrice)
	}

	if t.To != nil {
		addr := *t.To
		tt.To = &addr
	}

	tt.Value = new(big.Int)
	if t.Value != nil {
		tt.Value.Set(t.Value)
	}

	tt.Input = make([]byte, len(t.Input))
	copy(tt.Input[:], t.Input[:])

	if t.V != nil {
		tt.V = new(big.Int)
		tt.V = big.NewInt(0).SetBits(t.V.Bits())
	}

	if t.R != nil {
		tt.R = new(big.Int)
		tt.R = big.NewInt(0).SetBits(t.R.Bits())
	}

	if t.S != nil {
		tt.S = new(big.Int)
		tt.S = big.NewInt(0).SetBits(t.S.Bits())
	}

	return tt
}

func (t *LegacyTransaction) SetSignatureValues(v, r, s *big.Int) {
	t.V = v
	t.R = r
	t.S = s
}

func (t *LegacyTransaction) GetSignatureValues() (v, r, s *big.Int) {
	return t.V, t.R, t.S
}

type StateTransaction struct {
	Nonce      uint64
	To         *Address
	Input      []byte
	Signatures [][]byte // Validator Signatures
	V          *big.Int
	R          *big.Int
	S          *big.Int
}

func (t *StateTransaction) txType() TransactionType {
	return TxTypeState
}

func (t *StateTransaction) nonce() uint64 {
	return t.Nonce
}

func (t *StateTransaction) from() Address {
	return ZeroAddress
}

func (t *StateTransaction) setFrom(_from Address) {
	// do nothing
}

func (t *StateTransaction) to() *Address {
	return t.To
}

func (t *StateTransaction) value() *big.Int {
	return big.NewInt(0)
}

func (t *StateTransaction) input() []byte {
	return t.Input
}

func (t *StateTransaction) gas() uint64 {
	// unable to set gas manually
	return 0
}

func (t *StateTransaction) setGas(gas uint64) {}

func (t *StateTransaction) gasPrice() *big.Int {
	// StateTx will not go to TxPool and fee won't be paid to miner
	return big.NewInt(0)
}

func (t *StateTransaction) isContractCreation() bool {
	// StateTransaction doesn't support contract creation
	return false
}

func (t *StateTransaction) exceedsBlockGasLimit(_blockGasLimit uint64) bool {
	// No Block Gas Limit for State Transaction right now
	return false
}

func (t *StateTransaction) copy() TxPayload {
	tt := new(StateTransaction)
	*tt = *t

	if t.To != nil {
		addr := *t.To
		tt.To = &addr
	}

	tt.Input = make([]byte, len(t.Input))
	copy(tt.Input[:], t.Input[:])

	if t.Signatures != nil {
		tt.Signatures = make([][]byte, len(t.Signatures))

		for i := range t.Signatures {
			tt.Signatures[i] = make([]byte, len(t.Signatures[i]))
			copy(tt.Signatures[i][:], t.Signatures[i][:])
		}
	}

	if t.V != nil {
		tt.V = new(big.Int)
		tt.V = big.NewInt(0).SetBits(t.V.Bits())
	}

	if t.R != nil {
		tt.R = new(big.Int)
		tt.R = big.NewInt(0).SetBits(t.R.Bits())
	}

	if t.S != nil {
		tt.S = new(big.Int)
		tt.S = big.NewInt(0).SetBits(t.S.Bits())
	}

	return tt
}
