//
// Copyright Coinbase, Inc. All Rights Reserved.
//
// SPDX-License-Identifier: Apache-2.0
//

package fq

import (
	"math/big"
	"sync"

	"github.com/coinbase/kryptology/pkg/core/curves/native"
)

var k256FqInitonce sync.Once
var k256FqParams native.FieldParams

func K256FqNew() *native.Field {
	return &native.Field{
		Value:      [native.FieldLimbs]uint64{},
		Params:     getK256FqParams(),
		Arithmetic: k256FqArithmetic{},
	}
}

func k256FqParamsInit() {
	k256FqParams = native.FieldParams{
		R:       [native.FieldLimbs]uint64{0x402da1732fc9bebf, 0x4551231950b75fc4, 0x0000000000000001, 0x0000000000000000},
		R2:      [native.FieldLimbs]uint64{0x896cf21467d7d140, 0x741496c20e7cf878, 0xe697f5e45bcd07c6, 0x9d671cd581c69bc5},
		R3:      [native.FieldLimbs]uint64{0x7bc0cfe0e9ff41ed, 0x0017648444d4322c, 0xb1b31347f1d0b2da, 0x555d800c18ef116d},
		Modulus: [native.FieldLimbs]uint64{0xbfd25e8cd0364141, 0xbaaedce6af48a03b, 0xfffffffffffffffe, 0xffffffffffffffff},
		BiModulus: new(big.Int).SetBytes([]byte{
			0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xfe, 0xba, 0xae, 0xdc, 0xe6, 0xaf, 0x48, 0xa0, 0x3b, 0xbf, 0xd2, 0x5e, 0x8c, 0xd0, 0x36, 0x41, 0x41},
		),
	}
}

func getK256FqParams() *native.FieldParams {
	k256FqInitonce.Do(k256FqParamsInit)
	return &k256FqParams
}

// k256FqArithmetic is a struct with all the methods needed for working
// in mod q
type k256FqArithmetic struct{}

// ToMontgomery converts this field to montgomery form
func (f k256FqArithmetic) ToMontgomery(out, arg *[native.FieldLimbs]uint64) {
	ToMontgomery((*MontgomeryDomainFieldElement)(out), (*NonMontgomeryDomainFieldElement)(arg))
}

// FromMontgomery converts this field from montgomery form
func (f k256FqArithmetic) FromMontgomery(out, arg *[native.FieldLimbs]uint64) {
	FromMontgomery((*NonMontgomeryDomainFieldElement)(out), (*MontgomeryDomainFieldElement)(arg))
}

// Neg performs modular negation
func (f k256FqArithmetic) Neg(out, arg *[native.FieldLimbs]uint64) {
	Opp((*MontgomeryDomainFieldElement)(out), (*MontgomeryDomainFieldElement)(arg))
}

// Square performs modular square
func (f k256FqArithmetic) Square(out, arg *[native.FieldLimbs]uint64) {
	Square((*MontgomeryDomainFieldElement)(out), (*MontgomeryDomainFieldElement)(arg))
}

// Mul performs modular multiplication
func (f k256FqArithmetic) Mul(out, arg1, arg2 *[native.FieldLimbs]uint64) {
	Mul((*MontgomeryDomainFieldElement)(out), (*MontgomeryDomainFieldElement)(arg1), (*MontgomeryDomainFieldElement)(arg2))
}

// Add performs modular addition
func (f k256FqArithmetic) Add(out, arg1, arg2 *[native.FieldLimbs]uint64) {
	Add((*MontgomeryDomainFieldElement)(out), (*MontgomeryDomainFieldElement)(arg1), (*MontgomeryDomainFieldElement)(arg2))
}

// Sub performs modular subtraction
func (f k256FqArithmetic) Sub(out, arg1, arg2 *[native.FieldLimbs]uint64) {
	Sub((*MontgomeryDomainFieldElement)(out), (*MontgomeryDomainFieldElement)(arg1), (*MontgomeryDomainFieldElement)(arg2))
}

// Sqrt performs modular square root
func (f k256FqArithmetic) Sqrt(wasSquare *int, out, arg *[native.FieldLimbs]uint64) {
	// See sqrt_ts_ct at
	// https://datatracker.ietf.org/doc/html/draft-irtf-cfrg-hash-to-curve-11#appendix-I.4
	// c1 := 6
	// c2 := (q - 1) / (2^c1)
	// c2 := [4]uint64{
	//	0xeeff497a3340d905,
	//	0xfaeabb739abd2280,
	//	0xffffffffffffffff,
	//	0x03ffffffffffffff,
	//}
	// c3 := (c2 - 1) / 2
	c3 := [native.FieldLimbs]uint64{
		0x777fa4bd19a06c82,
		0xfd755db9cd5e9140,
		0xffffffffffffffff,
		0x01ffffffffffffff,
	}
	//c4 := generator
	//c5 := new(Fq).pow(generator, c2)
	c5 := [native.FieldLimbs]uint64{0x944cf2a220910e04, 0x815c829c780589f4, 0x55980b07bc222113, 0xc702b0d248825b36}
	var z, t, b, c, tv [native.FieldLimbs]uint64

	native.Pow(&z, arg, &c3, getK256FqParams(), f)
	Square((*MontgomeryDomainFieldElement)(&t), (*MontgomeryDomainFieldElement)(&z))
	Mul((*MontgomeryDomainFieldElement)(&t), (*MontgomeryDomainFieldElement)(&t), (*MontgomeryDomainFieldElement)(arg))
	Mul((*MontgomeryDomainFieldElement)(&z), (*MontgomeryDomainFieldElement)(&z), (*MontgomeryDomainFieldElement)(arg))

	copy(b[:], t[:])
	copy(c[:], c5[:])

	for i := s; i >= 2; i-- {
		for j := 1; j <= i-2; j++ {
			Square((*MontgomeryDomainFieldElement)(&b), (*MontgomeryDomainFieldElement)(&b))
		}
		// if b == 1 flag = 0 else flag = 1
		flag := -(&native.Field{
			Value:      b,
			Params:     getK256FqParams(),
			Arithmetic: f,
		}).IsOne() + 1
		Mul((*MontgomeryDomainFieldElement)(&tv), (*MontgomeryDomainFieldElement)(&z), (*MontgomeryDomainFieldElement)(&c))
		Selectznz(&z, uint1(flag), &z, &tv)
		Square((*MontgomeryDomainFieldElement)(&c), (*MontgomeryDomainFieldElement)(&c))
		Mul((*MontgomeryDomainFieldElement)(&tv), (*MontgomeryDomainFieldElement)(&t), (*MontgomeryDomainFieldElement)(&c))
		Selectznz(&t, uint1(flag), &t, &tv)
		copy(b[:], t[:])
	}
	Square((*MontgomeryDomainFieldElement)(&c), (*MontgomeryDomainFieldElement)(&z))
	*wasSquare = (&native.Field{
		Value:      c,
		Params:     getK256FqParams(),
		Arithmetic: f,
	}).Equal(&native.Field{
		Value:      *arg,
		Params:     getK256FqParams(),
		Arithmetic: f,
	})
	Selectznz(out, uint1(*wasSquare), out, &z)
}

// Invert performs modular inverse
func (f k256FqArithmetic) Invert(wasInverted *int, out, arg *[native.FieldLimbs]uint64) {
	// Using an addition chain from
	// https://briansmith.org/ecc-inversion-addition-chains-01#secp256k1_scalar_inversion
	var x1, x10, x11, x101, x111, x1001, x1011, x1101 [native.FieldLimbs]uint64
	var x6, x8, x14, x28, x56, tmp [native.FieldLimbs]uint64

	copy(x1[:], arg[:])
	native.Pow2k(&x10, arg, 1, f)
	Mul((*MontgomeryDomainFieldElement)(&x11), (*MontgomeryDomainFieldElement)(&x10), (*MontgomeryDomainFieldElement)(&x1))
	Mul((*MontgomeryDomainFieldElement)(&x101), (*MontgomeryDomainFieldElement)(&x10), (*MontgomeryDomainFieldElement)(&x11))
	Mul((*MontgomeryDomainFieldElement)(&x111), (*MontgomeryDomainFieldElement)(&x10), (*MontgomeryDomainFieldElement)(&x101))
	Mul((*MontgomeryDomainFieldElement)(&x1001), (*MontgomeryDomainFieldElement)(&x10), (*MontgomeryDomainFieldElement)(&x111))
	Mul((*MontgomeryDomainFieldElement)(&x1011), (*MontgomeryDomainFieldElement)(&x10), (*MontgomeryDomainFieldElement)(&x1001))
	Mul((*MontgomeryDomainFieldElement)(&x1101), (*MontgomeryDomainFieldElement)(&x10), (*MontgomeryDomainFieldElement)(&x1011))

	native.Pow2k(&x6, &x1101, 2, f)
	Mul((*MontgomeryDomainFieldElement)(&x6), (*MontgomeryDomainFieldElement)(&x6), (*MontgomeryDomainFieldElement)(&x1011))

	native.Pow2k(&x8, &x6, 2, f)
	Mul((*MontgomeryDomainFieldElement)(&x8), (*MontgomeryDomainFieldElement)(&x8), (*MontgomeryDomainFieldElement)(&x11))

	native.Pow2k(&x14, &x8, 6, f)
	Mul((*MontgomeryDomainFieldElement)(&x14), (*MontgomeryDomainFieldElement)(&x14), (*MontgomeryDomainFieldElement)(&x6))

	native.Pow2k(&x28, &x14, 14, f)
	Mul((*MontgomeryDomainFieldElement)(&x28), (*MontgomeryDomainFieldElement)(&x28), (*MontgomeryDomainFieldElement)(&x14))

	native.Pow2k(&x56, &x28, 28, f)
	Mul((*MontgomeryDomainFieldElement)(&x56), (*MontgomeryDomainFieldElement)(&x56), (*MontgomeryDomainFieldElement)(&x28))

	native.Pow2k(&tmp, &x56, 56, f)
	Mul((*MontgomeryDomainFieldElement)(&tmp), (*MontgomeryDomainFieldElement)(&tmp), (*MontgomeryDomainFieldElement)(&x56))

	native.Pow2k(&tmp, &tmp, 14, f)
	Mul((*MontgomeryDomainFieldElement)(&tmp), (*MontgomeryDomainFieldElement)(&tmp), (*MontgomeryDomainFieldElement)(&x14))

	native.Pow2k(&tmp, &tmp, 3, f)
	Mul((*MontgomeryDomainFieldElement)(&tmp), (*MontgomeryDomainFieldElement)(&tmp), (*MontgomeryDomainFieldElement)(&x101))

	native.Pow2k(&tmp, &tmp, 4, f)
	Mul((*MontgomeryDomainFieldElement)(&tmp), (*MontgomeryDomainFieldElement)(&tmp), (*MontgomeryDomainFieldElement)(&x111))

	native.Pow2k(&tmp, &tmp, 4, f)
	Mul((*MontgomeryDomainFieldElement)(&tmp), (*MontgomeryDomainFieldElement)(&tmp), (*MontgomeryDomainFieldElement)(&x101))

	native.Pow2k(&tmp, &tmp, 5, f)
	Mul((*MontgomeryDomainFieldElement)(&tmp), (*MontgomeryDomainFieldElement)(&tmp), (*MontgomeryDomainFieldElement)(&x1011))

	native.Pow2k(&tmp, &tmp, 4, f)
	Mul((*MontgomeryDomainFieldElement)(&tmp), (*MontgomeryDomainFieldElement)(&tmp), (*MontgomeryDomainFieldElement)(&x1011))

	native.Pow2k(&tmp, &tmp, 4, f)
	Mul((*MontgomeryDomainFieldElement)(&tmp), (*MontgomeryDomainFieldElement)(&tmp), (*MontgomeryDomainFieldElement)(&x111))

	native.Pow2k(&tmp, &tmp, 5, f)
	Mul((*MontgomeryDomainFieldElement)(&tmp), (*MontgomeryDomainFieldElement)(&tmp), (*MontgomeryDomainFieldElement)(&x111))

	native.Pow2k(&tmp, &tmp, 6, f)
	Mul((*MontgomeryDomainFieldElement)(&tmp), (*MontgomeryDomainFieldElement)(&tmp), (*MontgomeryDomainFieldElement)(&x1101))

	native.Pow2k(&tmp, &tmp, 4, f)
	Mul((*MontgomeryDomainFieldElement)(&tmp), (*MontgomeryDomainFieldElement)(&tmp), (*MontgomeryDomainFieldElement)(&x101))

	native.Pow2k(&tmp, &tmp, 3, f)
	Mul((*MontgomeryDomainFieldElement)(&tmp), (*MontgomeryDomainFieldElement)(&tmp), (*MontgomeryDomainFieldElement)(&x111))

	native.Pow2k(&tmp, &tmp, 5, f)
	Mul((*MontgomeryDomainFieldElement)(&tmp), (*MontgomeryDomainFieldElement)(&tmp), (*MontgomeryDomainFieldElement)(&x1001))

	native.Pow2k(&tmp, &tmp, 6, f)
	Mul((*MontgomeryDomainFieldElement)(&tmp), (*MontgomeryDomainFieldElement)(&tmp), (*MontgomeryDomainFieldElement)(&x101))

	native.Pow2k(&tmp, &tmp, 10, f)
	Mul((*MontgomeryDomainFieldElement)(&tmp), (*MontgomeryDomainFieldElement)(&tmp), (*MontgomeryDomainFieldElement)(&x111))

	native.Pow2k(&tmp, &tmp, 4, f)
	Mul((*MontgomeryDomainFieldElement)(&tmp), (*MontgomeryDomainFieldElement)(&tmp), (*MontgomeryDomainFieldElement)(&x111))

	native.Pow2k(&tmp, &tmp, 9, f)
	Mul((*MontgomeryDomainFieldElement)(&tmp), (*MontgomeryDomainFieldElement)(&tmp), (*MontgomeryDomainFieldElement)(&x8))

	native.Pow2k(&tmp, &tmp, 5, f)
	Mul((*MontgomeryDomainFieldElement)(&tmp), (*MontgomeryDomainFieldElement)(&tmp), (*MontgomeryDomainFieldElement)(&x1001))

	native.Pow2k(&tmp, &tmp, 6, f)
	Mul((*MontgomeryDomainFieldElement)(&tmp), (*MontgomeryDomainFieldElement)(&tmp), (*MontgomeryDomainFieldElement)(&x1011))

	native.Pow2k(&tmp, &tmp, 4, f)
	Mul((*MontgomeryDomainFieldElement)(&tmp), (*MontgomeryDomainFieldElement)(&tmp), (*MontgomeryDomainFieldElement)(&x1101))

	native.Pow2k(&tmp, &tmp, 5, f)
	Mul((*MontgomeryDomainFieldElement)(&tmp), (*MontgomeryDomainFieldElement)(&tmp), (*MontgomeryDomainFieldElement)(&x11))

	native.Pow2k(&tmp, &tmp, 6, f)
	Mul((*MontgomeryDomainFieldElement)(&tmp), (*MontgomeryDomainFieldElement)(&tmp), (*MontgomeryDomainFieldElement)(&x1101))

	native.Pow2k(&tmp, &tmp, 10, f)
	Mul((*MontgomeryDomainFieldElement)(&tmp), (*MontgomeryDomainFieldElement)(&tmp), (*MontgomeryDomainFieldElement)(&x1101))

	native.Pow2k(&tmp, &tmp, 4, f)
	Mul((*MontgomeryDomainFieldElement)(&tmp), (*MontgomeryDomainFieldElement)(&tmp), (*MontgomeryDomainFieldElement)(&x1001))

	native.Pow2k(&tmp, &tmp, 6, f)
	Mul((*MontgomeryDomainFieldElement)(&tmp), (*MontgomeryDomainFieldElement)(&tmp), (*MontgomeryDomainFieldElement)(&x1))

	native.Pow2k(&tmp, &tmp, 8, f)
	Mul((*MontgomeryDomainFieldElement)(&tmp), (*MontgomeryDomainFieldElement)(&tmp), (*MontgomeryDomainFieldElement)(&x6))

	*wasInverted = (&native.Field{
		Value:      *arg,
		Params:     getK256FqParams(),
		Arithmetic: f,
	}).IsNonZero()
	Selectznz(out, uint1(*wasInverted), out, &tmp)
}

// FromBytes converts a little endian byte array into a field element
func (f k256FqArithmetic) FromBytes(out *[native.FieldLimbs]uint64, arg *[native.FieldBytes]byte) {
	FromBytes(out, arg)
}

// ToBytes converts a field element to a little endian byte array
func (f k256FqArithmetic) ToBytes(out *[native.FieldBytes]byte, arg *[native.FieldLimbs]uint64) {
	ToBytes(out, arg)
}

// Selectznz performs conditional select.
// selects arg1 if choice == 0 and arg2 if choice == 1
func (f k256FqArithmetic) Selectznz(out, arg1, arg2 *[native.FieldLimbs]uint64, choice int) {
	Selectznz(out, uint1(choice), arg1, arg2)
}

// generator = 7 mod q is a generator of the `q - 1` order multiplicative
// subgroup, or in other words a primitive element of the field.
// generator^t where t * 2^s + 1 = q
var generator = &[native.FieldLimbs]uint64{0xc13f6a264e843739, 0xe537f5b135039e5d, 0x0000000000000008, 0x0000000000000000}

// s satisfies the equation 2^s * t = q - 1 with t odd.
var s = 6
