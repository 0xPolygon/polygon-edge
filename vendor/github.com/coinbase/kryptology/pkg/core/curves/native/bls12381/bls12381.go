package bls12381

import (
	"math/bits"

	"github.com/coinbase/kryptology/pkg/core/curves/native"
)

var fqModulusBytes = [native.FieldBytes]byte{0x01, 0x00, 0x00, 0x00, 0xff, 0xff, 0xff, 0xff, 0xfe, 0x5b, 0xfe, 0xff, 0x02, 0xa4, 0xbd, 0x53, 0x05, 0xd8, 0xa1, 0x09, 0x08, 0xd8, 0x39, 0x33, 0x48, 0x7d, 0x9d, 0x29, 0x53, 0xa7, 0xed, 0x73}

const (
	// The BLS parameter x for BLS12-381 is -0xd201000000010000
	paramX               = uint64(0xd201000000010000)
	Limbs                = 6
	FieldBytes           = 48
	WideFieldBytes       = 96
	DoubleWideFieldBytes = 192
)

// mac Multiply and Accumulate - compute a + (b * c) + d, return the result and new carry
func mac(a, b, c, d uint64) (uint64, uint64) {
	hi, lo := bits.Mul64(b, c)
	carry2, carry := bits.Add64(a, d, 0)
	hi, _ = bits.Add64(hi, 0, carry)
	lo, carry = bits.Add64(lo, carry2, 0)
	hi, _ = bits.Add64(hi, 0, carry)

	return lo, hi
}

// adc Add w/Carry
func adc(x, y, carry uint64) (uint64, uint64) {
	sum := x + y + carry
	// The sum will overflow if both top bits are set (x & y) or if one of them
	// is (x | y), and a carry from the lower place happened. If such a carry
	// happens, the top bit will be 1 + 0 + 1 = 0 (&^ sum).
	carryOut := ((x & y) | ((x | y) &^ sum)) >> 63
	carryOut |= ((x & carry) | ((x | carry) &^ sum)) >> 63
	carryOut |= ((y & carry) | ((y | carry) &^ sum)) >> 63
	return sum, carryOut
}

// sbb Subtract with borrow
func sbb(x, y, borrow uint64) (uint64, uint64) {
	diff := x - (y + borrow)
	borrowOut := ((^x & y) | (^(x ^ y) & diff)) >> 63
	return diff, borrowOut
}
