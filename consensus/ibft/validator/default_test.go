package validator

import (
	"reflect"
	"strings"
	"testing"

	"github.com/0xPolygon/minimal/consensus/ibft"
	"github.com/0xPolygon/minimal/crypto"
	"github.com/0xPolygon/minimal/types"
)

var (
	testAddress  = "70524d664ffe731100208a0154e556f9bb679ae6"
	testAddress2 = "b37866a925bccd69cfa98d43b510f1d23d78a851"
)

func TestValidatorSet(t *testing.T) {
	testNewValidatorSet(t)
	testNormalValSet(t)
	testEmptyValSet(t)
	testStickyProposer(t)
	testAddAndRemoveValidator(t)
}

func testNewValidatorSet(t *testing.T) {
	var validators []ibft.Validator
	const ValCnt = 100

	// Create 100 validators with random addresses
	b := []byte{}
	for i := 0; i < ValCnt; i++ {
		key, _ := crypto.GenerateKey()
		addr := crypto.PubKeyToAddress(&key.PublicKey)
		val := New(addr)
		validators = append(validators, val)
		b = append(b, val.Address().Bytes()...)
	}

	// Create ValidatorSet
	valSet := NewSet(ExtractValidators(b), ibft.RoundRobin)
	if valSet == nil {
		t.Errorf("the validator byte array cannot be parsed")
		t.FailNow()
	}

	// Check validators sorting: should be in ascending order
	for i := 0; i < ValCnt-1; i++ {
		val := valSet.GetByIndex(uint64(i))
		nextVal := valSet.GetByIndex(uint64(i + 1))
		if strings.Compare(val.String(), nextVal.String()) >= 0 {
			t.Errorf("validator set is not sorted in ascending order")
		}
	}
}

func testNormalValSet(t *testing.T) {
	b1 := types.Hex2Bytes(testAddress)
	b2 := types.Hex2Bytes(testAddress2)
	addr1 := types.BytesToAddress(b1)
	addr2 := types.BytesToAddress(b2)
	val1 := New(addr1)
	val2 := New(addr2)

	valSet := newDefaultSet([]types.Address{addr1, addr2}, ibft.RoundRobin)
	if valSet == nil {
		t.Errorf("the format of validator set is invalid")
		t.FailNow()
	}

	// check size
	if size := valSet.Size(); size != 2 {
		t.Errorf("the size of validator set is wrong: have %v, want 2", size)
	}
	// test get by index
	if val := valSet.GetByIndex(uint64(0)); !reflect.DeepEqual(val, val1) {
		t.Errorf("validator mismatch: have %v, want %v", val, val1)
	}
	// test get by invalid index
	if val := valSet.GetByIndex(uint64(2)); val != nil {
		t.Errorf("validator mismatch: have %v, want nil", val)
	}
	// test get by address
	if _, val := valSet.GetByAddress(addr2); !reflect.DeepEqual(val, val2) {
		t.Errorf("validator mismatch: have %v, want %v", val, val2)
	}
	// test get by invalid address
	invalidAddr := types.StringToAddress("0x9535b2e7faaba5288511d89341d94a38063a349b")
	if _, val := valSet.GetByAddress(invalidAddr); val != nil {
		t.Errorf("validator mismatch: have %v, want nil", val)
	}
	// test get proposer
	if val := valSet.GetProposer(); !reflect.DeepEqual(val, val1) {
		t.Errorf("proposer mismatch: have %v, want %v", val, val1)
	}
	// test calculate proposer
	lastProposer := addr1
	valSet.CalcProposer(lastProposer, uint64(0))
	if val := valSet.GetProposer(); !reflect.DeepEqual(val, val2) {
		t.Errorf("proposer mismatch: have %v, want %v", val, val2)
	}
	valSet.CalcProposer(lastProposer, uint64(3))
	if val := valSet.GetProposer(); !reflect.DeepEqual(val, val1) {
		t.Errorf("proposer mismatch: have %v, want %v", val, val1)
	}
	// test empty last proposer
	lastProposer = types.Address{}
	valSet.CalcProposer(lastProposer, uint64(3))
	if val := valSet.GetProposer(); !reflect.DeepEqual(val, val2) {
		t.Errorf("proposer mismatch: have %v, want %v", val, val2)
	}
}

func testEmptyValSet(t *testing.T) {
	valSet := NewSet(ExtractValidators([]byte{}), ibft.RoundRobin)
	if valSet == nil {
		t.Errorf("validator set should not be nil")
	}
}

func testAddAndRemoveValidator(t *testing.T) {
	valSet := NewSet(ExtractValidators([]byte{}), ibft.RoundRobin)
	if !valSet.AddValidator(types.StringToAddress(string(2))) {
		t.Error("the validator should be added")
	}
	if valSet.AddValidator(types.StringToAddress(string(2))) {
		t.Error("the existing validator should not be added")
	}
	valSet.AddValidator(types.StringToAddress(string(1)))
	valSet.AddValidator(types.StringToAddress(string(0)))
	if len(valSet.List()) != 3 {
		t.Error("the size of validator set should be 3")
	}

	for i, v := range valSet.List() {
		expected := types.StringToAddress(string(i))
		if v.Address() != expected {
			t.Errorf("the order of validators is wrong: have %v, want %v", v.Address().String(), expected.String())
		}
	}

	if !valSet.RemoveValidator(types.StringToAddress(string(2))) {
		t.Error("the validator should be removed")
	}
	if valSet.RemoveValidator(types.StringToAddress(string(2))) {
		t.Error("the non-existing validator should not be removed")
	}
	if len(valSet.List()) != 2 {
		t.Error("the size of validator set should be 2")
	}
	valSet.RemoveValidator(types.StringToAddress(string(1)))
	if len(valSet.List()) != 1 {
		t.Error("the size of validator set should be 1")
	}
	valSet.RemoveValidator(types.StringToAddress(string(0)))
	if len(valSet.List()) != 0 {
		t.Error("the size of validator set should be 0")
	}
}

func testStickyProposer(t *testing.T) {
	b1 := types.Hex2Bytes(testAddress)
	b2 := types.Hex2Bytes(testAddress2)
	addr1 := types.BytesToAddress(b1)
	addr2 := types.BytesToAddress(b2)
	val1 := New(addr1)
	val2 := New(addr2)

	valSet := newDefaultSet([]types.Address{addr1, addr2}, ibft.Sticky)

	// test get proposer
	if val := valSet.GetProposer(); !reflect.DeepEqual(val, val1) {
		t.Errorf("proposer mismatch: have %v, want %v", val, val1)
	}
	// test calculate proposer
	lastProposer := addr1
	valSet.CalcProposer(lastProposer, uint64(0))
	if val := valSet.GetProposer(); !reflect.DeepEqual(val, val1) {
		t.Errorf("proposer mismatch: have %v, want %v", val, val1)
	}

	valSet.CalcProposer(lastProposer, uint64(1))
	if val := valSet.GetProposer(); !reflect.DeepEqual(val, val2) {
		t.Errorf("proposer mismatch: have %v, want %v", val, val2)
	}
	// test empty last proposer
	lastProposer = types.Address{}
	valSet.CalcProposer(lastProposer, uint64(3))
	if val := valSet.GetProposer(); !reflect.DeepEqual(val, val2) {
		t.Errorf("proposer mismatch: have %v, want %v", val, val2)
	}
}
