package types

import (
	"fmt"
	"math/big"

	"github.com/umbracle/fastrlp"
)

type StateTx struct {
	Nonce    uint64
	GasPrice *big.Int
	Gas      uint64
	To       *Address
	Value    *big.Int
	Input    []byte
	V, R, S  *big.Int
	From     Address
	Hash     Hash
}

func (tx *StateTx) transactionType() TxType { return StateTxType }
func (tx *StateTx) chainID() *big.Int       { return nil }
func (tx *StateTx) input() []byte           { return tx.Input }
func (tx *StateTx) gas() uint64             { return tx.Gas }
func (tx *StateTx) gasPrice() *big.Int      { return tx.GasPrice }
func (tx *StateTx) gasTipCap() *big.Int     { return tx.GasPrice }
func (tx *StateTx) gasFeeCap() *big.Int     { return tx.GasPrice }
func (tx *StateTx) value() *big.Int         { return tx.Value }
func (tx *StateTx) nonce() uint64           { return tx.Nonce }
func (tx *StateTx) to() *Address            { return tx.To }
func (tx *StateTx) from() Address           { return tx.From }

func (tx *StateTx) hash() Hash { return tx.Hash }

func (tx *StateTx) rawSignatureValues() (v, r, s *big.Int) {
	return tx.V, tx.R, tx.S
}

func (tx *StateTx) accessList() TxAccessList {
	return nil
}

// set methods for transaction fields
func (tx *StateTx) setSignatureValues(v, r, s *big.Int) {
	tx.V, tx.R, tx.S = v, r, s
}

func (tx *StateTx) setFrom(addr Address) {
	tx.From = addr
}

func (tx *StateTx) setGas(gas uint64) {
	tx.Gas = gas
}

func (tx *StateTx) setChainID(id *big.Int) {}

func (tx *StateTx) setGasPrice(gas *big.Int) {
	tx.GasPrice = gas
}

func (tx *StateTx) setGasFeeCap(gas *big.Int) {
	tx.GasPrice = gas
}

func (tx *StateTx) setGasTipCap(gas *big.Int) {
	tx.GasPrice = gas
}

func (tx *StateTx) setValue(value *big.Int) {
	tx.Value = value
}

func (tx *StateTx) setInput(input []byte) {
	tx.Input = input
}

func (tx *StateTx) setTo(addeess *Address) {
	tx.To = addeess
}

func (tx *StateTx) setNonce(nonce uint64) {
	tx.Nonce = nonce
}

func (tx *StateTx) setAccessList(accessList TxAccessList) {}

func (tx *StateTx) setHash(h Hash) { tx.Hash = h }

// unmarshalRLPFrom unmarshals a Transaction in RLP format
// Be careful! This function does not de-serialize tx type, it assumes that t.Type is already set
// Hash calculation should also be done from the outside!
// Use UnmarshalRLP in most cases
func (tx *StateTx) unmarshalRLPFrom(p *fastrlp.Parser, v *fastrlp.Value) error {
	numOfElems := 10

	var (
		values rlpValues
		err    error
	)

	values, err = v.GetElems()
	if err != nil {
		return err
	}

	if numElems := len(values); numElems != numOfElems {
		return fmt.Errorf("incorrect number of transaction elements, expected %d but found %d", numOfElems, numElems)
	}

	// nonce
	txNonce, err := values.dequeueValue().GetUint64()
	if err != nil {
		return err
	}

	tx.setNonce(txNonce)

	// gasPrice
	txGasPrice := new(big.Int)
	if err = values.dequeueValue().GetBigInt(txGasPrice); err != nil {
		return err
	}

	tx.setGasPrice(txGasPrice)

	// gas
	txGas, err := values.dequeueValue().GetUint64()
	if err != nil {
		return err
	}

	tx.setGas(txGas)

	// to
	if vv, _ := values.dequeueValue().Bytes(); len(vv) == 20 {
		// address
		addr := BytesToAddress(vv)
		tx.setTo(&addr)
	} else {
		// reset To
		tx.setTo(nil)
	}

	// value
	txValue := new(big.Int)
	if err = values.dequeueValue().GetBigInt(txValue); err != nil {
		return err
	}

	tx.setValue(txValue)

	// input
	var txInput []byte

	txInput, err = values.dequeueValue().GetBytes(txInput)
	if err != nil {
		return err
	}

	tx.setInput(txInput)

	// V
	txV := new(big.Int)
	if err = values.dequeueValue().GetBigInt(txV); err != nil {
		return err
	}

	// R
	txR := new(big.Int)
	if err = values.dequeueValue().GetBigInt(txR); err != nil {
		return err
	}

	// S
	txS := new(big.Int)
	if err = values.dequeueValue().GetBigInt(txS); err != nil {
		return err
	}

	tx.setSignatureValues(txV, txR, txS)

	tx.setFrom(ZeroAddress)

	// We need to set From field for state transaction,
	// because we are using unique, predefined address, for sending such transactions
	if vv, err := values.dequeueValue().Bytes(); err == nil && len(vv) == AddressLength {
		// address
		tx.setFrom(BytesToAddress(vv))
	}

	return nil
}

// MarshalRLPWith marshals the transaction to RLP with a specific fastrlp.Arena
// Be careful! This function does not serialize tx type as a first byte.
// Use MarshalRLP/MarshalRLPTo in most cases
func (tx *StateTx) marshalRLPWith(arena *fastrlp.Arena) *fastrlp.Value {
	vv := arena.NewArray()

	vv.Set(arena.NewUint(tx.nonce()))
	vv.Set(arena.NewBigInt(tx.gasPrice()))
	vv.Set(arena.NewUint(tx.gas()))
	// Address may be empty
	if tx.to() != nil {
		vv.Set(arena.NewCopyBytes(tx.to().Bytes()))
	} else {
		vv.Set(arena.NewNull())
	}

	vv.Set(arena.NewBigInt(tx.value()))
	vv.Set(arena.NewCopyBytes(tx.input()))

	// signature values
	v, r, s := tx.rawSignatureValues()
	vv.Set(arena.NewBigInt(v))
	vv.Set(arena.NewBigInt(r))
	vv.Set(arena.NewBigInt(s))

	vv.Set(arena.NewCopyBytes(tx.from().Bytes()))

	return vv
}

func (tx *StateTx) copy() TxData { //nolint:dupl
	cpy := &StateTx{}

	cpy.setNonce(tx.nonce())

	if tx.gasPrice() != nil {
		gasPrice := new(big.Int)
		gasPrice.Set(tx.gasPrice())

		cpy.setGasPrice(gasPrice)
	}

	cpy.setGas(tx.gas())

	cpy.setTo(tx.to())

	if tx.value() != nil {
		value := new(big.Int)
		value.Set(tx.value())

		cpy.setValue(value)
	}

	inputCopy := make([]byte, len(tx.input()))
	copy(inputCopy, tx.input()[:])

	cpy.setInput(inputCopy)

	v, r, s := tx.rawSignatureValues()

	var vCopy, rCopy, sCopy *big.Int

	if v != nil {
		vCopy = new(big.Int)
		vCopy.Set(v)
	}

	if r != nil {
		rCopy = new(big.Int)
		rCopy.Set(r)
	}

	if s != nil {
		sCopy = new(big.Int)
		sCopy.Set(s)
	}

	cpy.setSignatureValues(vCopy, rCopy, sCopy)

	cpy.setFrom(tx.from())

	cpy.setHash(tx.hash())

	return cpy
}
