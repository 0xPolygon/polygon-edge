//
// Copyright Coinbase, Inc. All Rights Reserved.
//
// SPDX-License-Identifier: Apache-2.0
//

package fp

import (
	"encoding/binary"
	"fmt"
	"math/big"

	"github.com/coinbase/kryptology/internal"
)

type Fp fiat_pasta_fp_montgomery_domain_field_element

// r = 2^256 mod p
var r = &Fp{0x34786d38fffffffd, 0x992c350be41914ad, 0xffffffffffffffff, 0x3fffffffffffffff}

// r2 = 2^512 mod p
var r2 = &Fp{0x8c78ecb30000000f, 0xd7d30dbd8b0de0e7, 0x7797a99bc3c95d18, 0x096d41af7b9cb714}

// r3 = 2^768 mod p
var r3 = &Fp{0xf185a5993a9e10f9, 0xf6a68f3b6ac5b1d1, 0xdf8d1014353fd42c, 0x2ae309222d2d9910}

// generator = 5 mod p is a generator of the `p - 1` order multiplicative
// subgroup, or in other words a primitive element of the field.
var generator = &Fp{0xa1a55e68ffffffed, 0x74c2a54b4f4982f3, 0xfffffffffffffffd, 0x3fffffffffffffff}

var s = 32

// modulus representation
// p = 0x40000000000000000000000000000000224698fc094cf91b992d30ed00000001
var modulus = &Fp{0x992d30ed00000001, 0x224698fc094cf91b, 0x0000000000000000, 0x4000000000000000}

var biModulus = new(big.Int).SetBytes([]byte{
	0x40, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x22, 0x46, 0x98, 0xfc, 0x09, 0x4c, 0xf9, 0x1b,
	0x99, 0x2d, 0x30, 0xed, 0x00, 0x00, 0x00, 0x01,
})

// Cmp returns -1 if fp < rhs
// 0 if fp == rhs
// 1 if fp > rhs
func (fp *Fp) Cmp(rhs *Fp) int {
	gt := 0
	lt := 0
	for i := len(fp) - 1; i >= 0; i-- {
		gt |= int((rhs[i]-fp[i])>>63) &^ lt
		lt |= int((fp[i]-rhs[i])>>63) &^ gt
	}
	return gt - lt
}

// Equal returns true if fp == rhs
func (fp *Fp) Equal(rhs *Fp) bool {
	t := fp[0] ^ rhs[0]
	t |= fp[1] ^ rhs[1]
	t |= fp[2] ^ rhs[2]
	t |= fp[3] ^ rhs[3]
	return t == 0
}

// IsZero returns true if fp == 0
func (fp *Fp) IsZero() bool {
	t := fp[0]
	t |= fp[1]
	t |= fp[2]
	t |= fp[3]
	return t == 0
}

// IsOne returns true if fp == R
func (fp *Fp) IsOne() bool {
	return fp.Equal(r)
}

func (fp *Fp) IsOdd() bool {
	tv := new(fiat_pasta_fp_non_montgomery_domain_field_element)
	fiat_pasta_fp_from_montgomery(tv, (*fiat_pasta_fp_montgomery_domain_field_element)(fp))
	return tv[0]&0x01 == 0x01
}

// Set fp == rhs
func (fp *Fp) Set(rhs *Fp) *Fp {
	fp[0] = rhs[0]
	fp[1] = rhs[1]
	fp[2] = rhs[2]
	fp[3] = rhs[3]
	return fp
}

// SetUint64 sets fp == rhs
func (fp *Fp) SetUint64(rhs uint64) *Fp {
	r := &fiat_pasta_fp_non_montgomery_domain_field_element{rhs, 0, 0, 0}
	fiat_pasta_fp_to_montgomery((*fiat_pasta_fp_montgomery_domain_field_element)(fp), r)
	return fp
}

func (fp *Fp) SetBool(rhs bool) *Fp {
	if rhs {
		fp.SetOne()
	} else {
		fp.SetZero()
	}
	return fp
}

// SetOne fp == R
func (fp *Fp) SetOne() *Fp {
	return fp.Set(r)
}

// SetZero fp == 0
func (fp *Fp) SetZero() *Fp {
	fp[0] = 0
	fp[1] = 0
	fp[2] = 0
	fp[3] = 0
	return fp
}

// SetBytesWide takes 64 bytes as input and treats them as a 512-bit number.
// Attributed to https://github.com/zcash/pasta_curves/blob/main/src/fields/fp.rs#L255
// We reduce an arbitrary 512-bit number by decomposing it into two 256-bit digits
// with the higher bits multiplied by 2^256. Thus, we perform two reductions
//
// 1. the lower bits are multiplied by R^2, as normal
// 2. the upper bits are multiplied by R^2 * 2^256 = R^3
//
// and computing their sum in the field. It remains to see that arbitrary 256-bit
// numbers can be placed into Montgomery form safely using the reduction. The
// reduction works so long as the product is less than R=2^256 multiplied by
// the modulus. This holds because for any `c` smaller than the modulus, we have
// that (2^256 - 1)*c is an acceptable product for the reduction. Therefore, the
// reduction always works so long as `c` is in the field; in this case it is either the
// constant `r2` or `r3`.
func (fp *Fp) SetBytesWide(input *[64]byte) *Fp {
	d0 := fiat_pasta_fp_montgomery_domain_field_element{
		binary.LittleEndian.Uint64(input[:8]),
		binary.LittleEndian.Uint64(input[8:16]),
		binary.LittleEndian.Uint64(input[16:24]),
		binary.LittleEndian.Uint64(input[24:32]),
	}
	d1 := fiat_pasta_fp_montgomery_domain_field_element{
		binary.LittleEndian.Uint64(input[32:40]),
		binary.LittleEndian.Uint64(input[40:48]),
		binary.LittleEndian.Uint64(input[48:56]),
		binary.LittleEndian.Uint64(input[56:64]),
	}
	// Convert to Montgomery form
	tv1 := new(fiat_pasta_fp_montgomery_domain_field_element)
	tv2 := new(fiat_pasta_fp_montgomery_domain_field_element)
	// d0 * r2 + d1 * r3
	fiat_pasta_fp_mul(tv1, &d0, (*fiat_pasta_fp_montgomery_domain_field_element)(r2))
	fiat_pasta_fp_mul(tv2, &d1, (*fiat_pasta_fp_montgomery_domain_field_element)(r3))
	fiat_pasta_fp_add((*fiat_pasta_fp_montgomery_domain_field_element)(fp), tv1, tv2)
	return fp
}

// SetBytes attempts to convert a little endian byte representation
// of a scalar into a `Fp`, failing if input is not canonical
func (fp *Fp) SetBytes(input *[32]byte) (*Fp, error) {
	d0 := &Fp{
		binary.LittleEndian.Uint64(input[:8]),
		binary.LittleEndian.Uint64(input[8:16]),
		binary.LittleEndian.Uint64(input[16:24]),
		binary.LittleEndian.Uint64(input[24:32]),
	}
	if d0.Cmp(modulus) != -1 {
		return nil, fmt.Errorf("invalid byte sequence")
	}
	fiat_pasta_fp_from_bytes((*[4]uint64)(fp), input)
	fiat_pasta_fp_to_montgomery((*fiat_pasta_fp_montgomery_domain_field_element)(fp), (*fiat_pasta_fp_non_montgomery_domain_field_element)(fp))
	return fp, nil
}

// SetBigInt initializes an element from big.Int
// The value is reduced by the modulus
func (fp *Fp) SetBigInt(bi *big.Int) *Fp {
	var buffer [32]byte
	r := new(big.Int).Set(bi)
	r.Mod(r, biModulus)
	r.FillBytes(buffer[:])
	copy(buffer[:], internal.ReverseScalarBytes(buffer[:]))
	_, _ = fp.SetBytes(&buffer)
	return fp
}

// SetRaw converts a raw array into a field element
func (fp *Fp) SetRaw(array *[4]uint64) *Fp {
	fiat_pasta_fp_to_montgomery((*fiat_pasta_fp_montgomery_domain_field_element)(fp), (*fiat_pasta_fp_non_montgomery_domain_field_element)(array))
	return fp
}

// Bytes converts this element into a byte representation
// in little endian byte order
func (fp *Fp) Bytes() [32]byte {
	var output [32]byte
	tv := new(fiat_pasta_fp_non_montgomery_domain_field_element)
	fiat_pasta_fp_from_montgomery(tv, (*fiat_pasta_fp_montgomery_domain_field_element)(fp))
	fiat_pasta_fp_to_bytes(&output, (*[4]uint64)(tv))
	return output
}

// BigInt converts this element into the big.Int struct
func (fp *Fp) BigInt() *big.Int {
	buffer := fp.Bytes()
	return new(big.Int).SetBytes(internal.ReverseScalarBytes(buffer[:]))
}

// Double this element
func (fp *Fp) Double(elem *Fp) *Fp {
	delem := (*fiat_pasta_fp_montgomery_domain_field_element)(elem)
	fiat_pasta_fp_add((*fiat_pasta_fp_montgomery_domain_field_element)(fp), delem, delem)
	return fp
}

// Square this element
func (fp *Fp) Square(elem *Fp) *Fp {
	delem := (*fiat_pasta_fp_montgomery_domain_field_element)(elem)
	fiat_pasta_fp_square((*fiat_pasta_fp_montgomery_domain_field_element)(fp), delem)
	return fp
}

// Sqrt this element, if it exists. If true, then value
// is a square root. If false, value is a QNR
func (fp *Fp) Sqrt(elem *Fp) (*Fp, bool) {
	return fp.tonelliShanks(elem)
}

// See sqrt_ts_ct at
// https://datatracker.ietf.org/doc/html/draft-irtf-cfrg-hash-to-curve-11#appendix-I.4
func (fp *Fp) tonelliShanks(elem *Fp) (*Fp, bool) {
	// c1 := 32
	// c2 := (q - 1) / (2^c1)
	// c2 := [4]uint64{
	// 	0x094cf91b992d30ed,
	// 	0x00000000224698fc,
	// 	0x0000000000000000,
	// 	0x0000000040000000,
	// }
	// c3 := (c2 - 1) / 2
	c3 := [4]uint64{
		0x04a67c8dcc969876,
		0x0000000011234c7e,
		0x0000000000000000,
		0x0000000020000000,
	}
	// c4 := generator
	// c5 := new(Fp).pow(&generator, c2)
	c5 := &Fp{
		0xa28db849bad6dbf0,
		0x9083cd03d3b539df,
		0xfba6b9ca9dc8448e,
		0x3ec928747b89c6da,
	}

	z := new(Fp).pow(elem, c3)
	t := new(Fp).Square(z)
	t.Mul(t, elem)

	z.Mul(z, elem)

	b := new(Fp).Set(t)
	c := new(Fp).Set(c5)
	flags := map[bool]int{
		true:  1,
		false: 0,
	}

	for i := s; i >= 2; i-- {
		for j := 1; j <= i-2; j++ {
			b.Square(b)
		}
		z.CMove(z, new(Fp).Mul(z, c), flags[!b.IsOne()])
		c.Square(c)
		t.CMove(t, new(Fp).Mul(t, c), flags[!b.IsOne()])
		b.Set(t)
	}
	wasSquare := c.Square(z).Equal(elem)
	return fp.Set(z), wasSquare
}

// Invert this element i.e. compute the multiplicative inverse
// return false, zero if this element is zero
func (fp *Fp) Invert(elem *Fp) (*Fp, bool) {
	// computes elem^(p - 2) mod p
	exp := [4]uint64{
		0x992d30ecffffffff,
		0x224698fc094cf91b,
		0x0000000000000000,
		0x4000000000000000}
	return fp.pow(elem, exp), !elem.IsZero()
}

// Mul returns the result from multiplying this element by rhs
func (fp *Fp) Mul(lhs, rhs *Fp) *Fp {
	dlhs := (*fiat_pasta_fp_montgomery_domain_field_element)(lhs)
	drhs := (*fiat_pasta_fp_montgomery_domain_field_element)(rhs)
	fiat_pasta_fp_mul((*fiat_pasta_fp_montgomery_domain_field_element)(fp), dlhs, drhs)
	return fp
}

// Sub returns the result from subtracting rhs from this element
func (fp *Fp) Sub(lhs, rhs *Fp) *Fp {
	dlhs := (*fiat_pasta_fp_montgomery_domain_field_element)(lhs)
	drhs := (*fiat_pasta_fp_montgomery_domain_field_element)(rhs)
	fiat_pasta_fp_sub((*fiat_pasta_fp_montgomery_domain_field_element)(fp), dlhs, drhs)
	return fp
}

// Add returns the result from adding rhs to this element
func (fp *Fp) Add(lhs, rhs *Fp) *Fp {
	dlhs := (*fiat_pasta_fp_montgomery_domain_field_element)(lhs)
	drhs := (*fiat_pasta_fp_montgomery_domain_field_element)(rhs)
	fiat_pasta_fp_add((*fiat_pasta_fp_montgomery_domain_field_element)(fp), dlhs, drhs)
	return fp
}

// Neg returns negation of this element
func (fp *Fp) Neg(elem *Fp) *Fp {
	delem := (*fiat_pasta_fp_montgomery_domain_field_element)(elem)
	fiat_pasta_fp_opp((*fiat_pasta_fp_montgomery_domain_field_element)(fp), delem)
	return fp
}

// Exp exponentiates this element by exp
func (fp *Fp) Exp(base, exp *Fp) *Fp {
	// convert exponent to integer form
	tv := &fiat_pasta_fp_non_montgomery_domain_field_element{}
	fiat_pasta_fp_from_montgomery(tv, (*fiat_pasta_fp_montgomery_domain_field_element)(exp))

	e := (*[4]uint64)(tv)
	return fp.pow(base, *e)
}

func (fp *Fp) pow(base *Fp, exp [4]uint64) *Fp {
	res := new(Fp).SetOne()
	tmp := new(Fp)

	for i := len(exp) - 1; i >= 0; i-- {
		for j := 63; j >= 0; j-- {
			res.Square(res)
			tmp.Mul(res, base)
			res.CMove(res, tmp, int(exp[i]>>j)&1)
		}
	}
	return fp.Set(res)
}

// CMove selects lhs if choice == 0 and rhs if choice == 1
func (fp *Fp) CMove(lhs, rhs *Fp, choice int) *Fp {
	dlhs := (*[4]uint64)(lhs)
	drhs := (*[4]uint64)(rhs)
	fiat_pasta_fp_selectznz((*[4]uint64)(fp), fiat_pasta_fp_uint1(choice), dlhs, drhs)
	return fp
}

// ToRaw converts this element into the a [4]uint64
func (fp *Fp) ToRaw() [4]uint64 {
	res := &fiat_pasta_fp_non_montgomery_domain_field_element{}
	fiat_pasta_fp_from_montgomery(res, (*fiat_pasta_fp_montgomery_domain_field_element)(fp))
	return *(*[4]uint64)(res)
}
