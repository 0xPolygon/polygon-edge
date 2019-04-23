package evm

import (
	"bytes"
	"fmt"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/umbracle/minimal/chain"
	"github.com/umbracle/minimal/state/runtime"
)

func mustDecode(s string) []byte {
	res, err := hexutil.Decode(s)
	if err != nil {
		panic(err)
	}
	return res
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

func TestStackOperators(t *testing.T) {
	cases := []struct {
		code []byte
		pre  []int
		post []int
	}{
		{
			code: []byte{byte(DUP1)},
			pre:  []int{1, 2, 3},
			post: []int{3, 3, 2, 1},
		},
		{
			code: []byte{byte(SWAP1)},
			pre:  []int{1, 2, 3},
			post: []int{2, 3, 1},
		},
		{
			code: []byte{byte(PUSH3), 0x1, 0x2, 0x3},
			post: []int{66051}, // 0x102030
		},
	}

	for _, c := range cases {
		t.Run("", func(t *testing.T) {
			contract := &Contract{
				ip:    -1,
				code:  Instructions(c.code),
				gas:   1000,
				stack: make([]*big.Int, StackSize),
			}
			if c.pre != nil {
				for _, b := range c.pre {
					contract.push(big.NewInt(int64(b)))
				}
			}

			if _, err := contract.Run(); err != nil {
				t.Fatal(err)
			}
			for indx, i := range c.post {
				val := contract.peekAt(indx + 1)
				if val.Int64() != int64(i) {
					t.Fatalf("at index %d expected %d but found %d", indx, i, val.Int64())
				}
			}
		})
	}
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
		{"ABCDEF0908070605040302010000000000000000000000000000000000000000", "00", "ab"},
		{"ABCDEF0908070605040302010000000000000000000000000000000000000000", "01", "cd"},
		{"00CDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff", "00", "00"},
		{"00CDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff", "01", "cd"},
		{"0000000000000000000000000000000000000000000000000000000000102030", "1f", "30"},
		{"0000000000000000000000000000000000000000000000000000000000102030", "1e", "20"},
		{"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "20", "00"},
		{"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "FFFFFFFFFFFFFFFF", "00"},
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
			c := newTestContract(Instructions{byte(instruction)})
			c.push(big.NewInt(cc.x))
			c.push(big.NewInt(cc.y))

			if _, err := c.Run(); err != nil {
				t.Fatal(err)
			}
			peek := c.peek()
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
			c := newTestContract(Instructions{byte(instruction)})
			c.evm = &EVM{
				config: chain.ForksInTime{Constantinople: true},
				env:    &runtime.Env{Number: big.NewInt(0)},
			}

			c.push(big.NewInt(1).SetBytes(mustDecode("0x" + cc.x)))
			c.push(big.NewInt(1).SetBytes(mustDecode("0x" + cc.y)))

			if _, err := c.Run(); err != nil {
				t.Fatal(err)
			}

			// remove 0x prefix and pad zeros to the left
			found := strings.Replace(hexutil.Encode(c.peek().Bytes()), "0x", "", -1)
			found = fmt.Sprintf("%064s", found)

			if !strings.Contains(found, cc.expected) {
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

func equalInt(t *testing.T, i, j uint32) {
	if i != j {
		t.Fatalf("mismatch: i (%d) j (%d)", i, j)
	}
}

func c(i int64) *big.Int {
	return big.NewInt(i)
}
