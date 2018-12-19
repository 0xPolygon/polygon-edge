package evm

import (
	"bytes"
	"fmt"
	"math/big"
	"strings"
	"testing"

	"github.com/umbracle/minimal/chain"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/ethdb"
)

func mustDecode(s string) []byte {
	res, err := hexutil.Decode(s)
	if err != nil {
		panic(err)
	}
	return res
}

func newState(t *testing.T) *state.StateDB {
	state, err := state.New(common.Hash{}, state.NewDatabase(ethdb.NewMemDatabase()))
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func newTestContract(code []byte) *Contract {
	f := &Contract{
		ip:    -1,
		code:  code,
		gas:   1000000,
		stack: make([]*big.Int, StackSize),
		sp:    0,
	}
	f.memory = newMemory(f)
	return f
}

func testEVM(code []byte) *EVM {
	evm := NewEVM(nil, nil, chain.ForksInTime{}, chain.GasTableHomestead, nil)
	evm.pushContract(newTestContract(code))
	return evm
}

func testEVMWithStack(stack []byte, op OpCode) *EVM {
	evm := NewEVM(nil, nil, chain.ForksInTime{}, chain.GasTableHomestead, nil)
	evm.pushContract(newTestContract([]byte{byte(op)}))
	for _, i := range stack {
		evm.push(big.NewInt(0).SetBytes([]byte{i}))
	}
	return evm
}

func testStack(t *testing.T, evm *EVM, items []int32) {
	for indx, i := range items {
		val := evm.peekAt(indx + 1)
		if val.Int64() != int64(i) {
			t.Fatalf("at index %d expected %d but found %d", indx, i, val.Int64())
		}
	}
}

func TestPushOperators(t *testing.T) {
	code := []byte{
		byte(PUSH3),
		0x1,
		0x2,
		0x3,
	}

	evm := testEVM(code)
	if err := evm.Run(); err != nil {
		t.Fatal(err)
	}

	testStack(t, evm, []int32{66051}) // 0x102030
}

func TestSwapOperators(t *testing.T) {
	stack := []byte{
		0x1,
		0x2,
		0x3,
	}

	evm := testEVMWithStack(stack, SWAP1)
	if err := evm.Run(); err != nil {
		t.Fatal(err)
	}

	testStack(t, evm, []int32{2, 3, 1})
}

func TestDupOperators(t *testing.T) {
	stack := []byte{
		0x1,
		0x2,
		0x3,
	}

	evm := testEVMWithStack(stack, DUP1)
	if err := evm.Run(); err != nil {
		t.Fatal(err)
	}

	testStack(t, evm, []int32{3, 3, 2, 1})
}

func TestArithmeticOperands(t *testing.T) {
	cases := []intTestCase{
		{1, 2, 3},
	}
	testIntTestCases(t, ADD, cases)

	cases = []intTestCase{
		{5, 3, -2},
		{3, 5, 2},
	}
	testIntTestCases(t, SUB, cases)

	cases = []intTestCase{
		{2, 2, 4},
	}
	testIntTestCases(t, MUL, cases)

	cases = []intTestCase{
		{4, 2, 0},
		{2, 4, 2},
	}
	testIntTestCases(t, DIV, cases)
}

func TestComparisonOperands(t *testing.T) {
	cases := []intTestCase{
		{1, 2, 0},
		{1, 1, 1},
	}
	testIntTestCases(t, EQ, cases)

	cases = []intTestCase{
		{0, 1, 1},
		{1, 0, 0},
		{0, 0, 0},
	}
	testIntTestCases(t, GT, cases)

	cases = []intTestCase{
		{1, 0, 1},
		{0, 1, 0},
		{0, 0, 0},
	}
	testIntTestCases(t, LT, cases)
}

func TestBitWiseLogic(t *testing.T) {
	cases := []stringTestCase{
		{"11", "01", "0000000000000000000000000000000000000000000000000000000000000001"},
		{"11", "11", "0000000000000000000000000000000000000000000000000000000000000011"},
		{"00", "00", "0000000000000000000000000000000000000000000000000000000000000000"},
	}
	testStringTestCases(t, AND, cases)

	cases = []stringTestCase{
		{"11", "01", "0000000000000000000000000000000000000000000000000000000000000011"},
		{"11", "11", "0000000000000000000000000000000000000000000000000000000000000011"},
		{"00", "00", "0000000000000000000000000000000000000000000000000000000000000000"},
	}
	testStringTestCases(t, OR, cases)

	cases = []stringTestCase{
		{"11", "01", "0000000000000000000000000000000000000000000000000000000000000010"},
		{"11", "11", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"00", "00", "0000000000000000000000000000000000000000000000000000000000000000"},
	}
	testStringTestCases(t, XOR, cases)

	cases = []stringTestCase{
		{"ABCD", "00", "AB"},
	}
	testStringTestCases(t, BYTE, cases)

	cases = []stringTestCase{
		{"00", "00", "0000000000000000000000000000000000000000000000000000000000000001"},
		{"00", "11", "0000000000000000000000000000000000000000000000000000000000000000"},
	}
	testStringTestCases(t, ISZERO, cases)
}

func TestShiftOperands(t *testing.T) {
	// Testcases from https://github.com/ethereum/EIPs/blob/master/EIPS/eip-145.md#shl-shift-left
	cases := []stringTestCase{
		{"0000000000000000000000000000000000000000000000000000000000000001", "00", "0000000000000000000000000000000000000000000000000000000000000001"},
		{"0000000000000000000000000000000000000000000000000000000000000001", "01", "0000000000000000000000000000000000000000000000000000000000000002"},
		{"0000000000000000000000000000000000000000000000000000000000000001", "ff", "8000000000000000000000000000000000000000000000000000000000000000"},
		{"0000000000000000000000000000000000000000000000000000000000000001", "0100", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"0000000000000000000000000000000000000000000000000000000000000001", "0101", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "00", "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"},
		{"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "01", "fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffe"},
		{"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "ff", "8000000000000000000000000000000000000000000000000000000000000000"},
		{"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "0100", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"0000000000000000000000000000000000000000000000000000000000000000", "01", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "01", "fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffe"},
	}
	testStringTestCases(t, SHL, cases)

	// Testcases from https://github.com/ethereum/EIPs/blob/master/EIPS/eip-145.md#shr-logical-shift-right
	cases = []stringTestCase{
		{"0000000000000000000000000000000000000000000000000000000000000001", "00", "0000000000000000000000000000000000000000000000000000000000000001"},
		{"0000000000000000000000000000000000000000000000000000000000000001", "01", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"8000000000000000000000000000000000000000000000000000000000000000", "01", "4000000000000000000000000000000000000000000000000000000000000000"},
		{"8000000000000000000000000000000000000000000000000000000000000000", "ff", "0000000000000000000000000000000000000000000000000000000000000001"},
		{"8000000000000000000000000000000000000000000000000000000000000000", "0100", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"8000000000000000000000000000000000000000000000000000000000000000", "0101", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "00", "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"},
		{"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "01", "7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"},
		{"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "ff", "0000000000000000000000000000000000000000000000000000000000000001"},
		{"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "0100", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"0000000000000000000000000000000000000000000000000000000000000000", "01", "0000000000000000000000000000000000000000000000000000000000000000"},
	}
	testStringTestCases(t, SHR, cases)

	// Testcases from https://github.com/ethereum/EIPs/blob/master/EIPS/eip-145.md#sar-arithmetic-shift-right
	cases = []stringTestCase{
		{"0000000000000000000000000000000000000000000000000000000000000001", "00", "0000000000000000000000000000000000000000000000000000000000000001"},
		{"0000000000000000000000000000000000000000000000000000000000000001", "01", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"8000000000000000000000000000000000000000000000000000000000000000", "01", "c000000000000000000000000000000000000000000000000000000000000000"},
		{"8000000000000000000000000000000000000000000000000000000000000000", "ff", "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"},
		{"8000000000000000000000000000000000000000000000000000000000000000", "0100", "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"},
		{"8000000000000000000000000000000000000000000000000000000000000000", "0101", "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"},
		{"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "00", "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"},
		{"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "01", "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"},
		{"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "ff", "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"},
		{"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "0100", "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"},
		{"0000000000000000000000000000000000000000000000000000000000000000", "01", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"4000000000000000000000000000000000000000000000000000000000000000", "fe", "0000000000000000000000000000000000000000000000000000000000000001"},
		{"7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "f8", "000000000000000000000000000000000000000000000000000000000000007f"},
		{"7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "fe", "0000000000000000000000000000000000000000000000000000000000000001"},
		{"7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "ff", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "0100", "0000000000000000000000000000000000000000000000000000000000000000"},
	}
	testStringTestCases(t, SAR, cases)
}

func TestSignedComparisonOperands(t *testing.T) {
	cases := []stringTestCase{
		{"0000000000000000000000000000000000000000000000000000000000000001", "0000000000000000000000000000000000000000000000000000000000000001", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"0000000000000000000000000000000000000000000000000000000000000001", "7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "0000000000000000000000000000000000000000000000000000000000000001"},
		{"7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "0000000000000000000000000000000000000000000000000000000000000001", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "0000000000000000000000000000000000000000000000000000000000000001", "0000000000000000000000000000000000000000000000000000000000000001"},
		{"0000000000000000000000000000000000000000000000000000000000000001", "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"8000000000000000000000000000000000000000000000000000000000000001", "8000000000000000000000000000000000000000000000000000000000000001", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"8000000000000000000000000000000000000000000000000000000000000001", "7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "0000000000000000000000000000000000000000000000000000000000000001"},
		{"7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "8000000000000000000000000000000000000000000000000000000000000001", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffb", "fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffd", "0000000000000000000000000000000000000000000000000000000000000001"},
		{"fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffd", "fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffb", "0000000000000000000000000000000000000000000000000000000000000000"},
	}
	testStringTestCases(t, SGT, cases)

	cases = []stringTestCase{
		{"0000000000000000000000000000000000000000000000000000000000000001", "0000000000000000000000000000000000000000000000000000000000000001", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"0000000000000000000000000000000000000000000000000000000000000001", "7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "0000000000000000000000000000000000000000000000000000000000000001", "0000000000000000000000000000000000000000000000000000000000000001"},
		{"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "0000000000000000000000000000000000000000000000000000000000000001", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"0000000000000000000000000000000000000000000000000000000000000001", "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "0000000000000000000000000000000000000000000000000000000000000001"},
		{"8000000000000000000000000000000000000000000000000000000000000001", "8000000000000000000000000000000000000000000000000000000000000001", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"8000000000000000000000000000000000000000000000000000000000000001", "7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "8000000000000000000000000000000000000000000000000000000000000001", "0000000000000000000000000000000000000000000000000000000000000001"},
		{"fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffb", "fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffd", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffd", "fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffb", "0000000000000000000000000000000000000000000000000000000000000001"},
	}
	testStringTestCases(t, SLT, cases)
}

type intTestCase struct {
	x, y, expected int64
}

func testIntTestCases(t *testing.T, instruction OpCode, cases []intTestCase) {
	for _, cc := range cases {
		t.Run(instruction.String(), func(t *testing.T) {
			evm := testEVM(Instructions{byte(instruction)})
			evm.push(big.NewInt(cc.x))
			evm.push(big.NewInt(cc.y))

			if err := evm.Run(); err != nil {
				t.Fatal(err)
			}

			peek := evm.peek()
			if peek.Int64() != cc.expected {
				t.Fatalf("expected %d but found %d", peek.Int64(), cc.expected)
			}
		})
	}
}

type stringTestCase struct {
	x, y, expected string
}

func testStringTestCases(t *testing.T, instruction OpCode, cases []stringTestCase) {
	for _, cc := range cases {
		t.Run(instruction.String(), func(t *testing.T) {
			evm := testEVM(Instructions{byte(instruction)})
			evm.config = chain.ForksInTime{Constantinople: true}
			evm.env = &Env{Number: big.NewInt(0)}

			evm.push(big.NewInt(1).SetBytes(mustDecode("0x" + cc.x)))
			evm.push(big.NewInt(1).SetBytes(mustDecode("0x" + cc.y)))

			if err := evm.Run(); err != nil {
				t.Fatal(err)
			}

			// remove 0x prefix and pad zeros to the left
			found := strings.Replace(hexutil.Encode(evm.peek().Bytes()), "0x", "", -1)
			found = fmt.Sprintf("%064s", found)

			if found != cc.expected {
				t.Fatalf("not equal. Found %s and expected %s", found, cc.expected)
			}
		})
	}
}

func equalBytes(t *testing.T, a, b []byte) {
	if !bytes.Equal(a, b) {
		t.Fatal("not equal")
	}
}

func expectLength(t *testing.T, m *Memory, len int) {
	if m.Len() != len {
		t.Fatalf("expected length %d but found %d", len, m.Len())
	}
}

func equalInt(t *testing.T, i, j uint32) {
	if i != j {
		t.Fatalf("mismatch: i (%d) j (%d)", i, j)
	}
}

func c(i int64) *big.Int {
	return big.NewInt(i)
}

func TestMemorySetResize(t *testing.T) {
	m := newMemory(newTestContract([]byte{}))
	data := mustDecode("0x123456")

	m.Set(c(0), c(3), data)
	expectLength(t, m, 32)

	equalBytes(t, m.store, common.RightPadBytes(data, 32))
	found, _, err := m.Get(c(0), c(3))
	if err != nil {
		t.Fatal(err)
	}
	equalBytes(t, found, data)

	// resize not necessary
	m.Set(c(10), c(3), data)
	expectLength(t, m, 32)

	m.Set(c(65), c(10), data)
	expectLength(t, m, 96)

	// take two more slots
	m.Set(c(129), c(65), data)
	expectLength(t, m, 224)
}

func TestMemorySetByte(t *testing.T) {
	m := newMemory(newTestContract([]byte{}))

	m.SetByte(c(10), 10)
	expectLength(t, m, 32)

	m.SetByte(c(31), 10)
	expectLength(t, m, 32)

	m.SetByte(c(32), 10)
	expectLength(t, m, 64)
}

func TestMemorySet32(t *testing.T) {
	m := newMemory(newTestContract([]byte{}))

	m.Set32(c(0), big.NewInt(32))
	expectLength(t, m, 32)

	m.Set32(c(1), big.NewInt(32))
	expectLength(t, m, 64)

	m = newMemory(newTestContract([]byte{}))
	m.Set32(c(0), big.NewInt(32))
	expectLength(t, m, 32)

	m.Set32(c(32), big.NewInt(32))
	expectLength(t, m, 64)
}
