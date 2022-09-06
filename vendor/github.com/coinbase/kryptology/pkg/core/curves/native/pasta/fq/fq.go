//
// Copyright Coinbase, Inc. All Rights Reserved.
//
// SPDX-License-Identifier: Apache-2.0
//

package fq

import (
	"encoding/binary"
	"fmt"
	"math/big"

	"github.com/coinbase/kryptology/internal"
)

type Fq fiat_pasta_fq_montgomery_domain_field_element

// r = 2^256 mod p
var r = &Fq{0x5b2b3e9cfffffffd, 0x992c350be3420567, 0xffffffffffffffff, 0x3fffffffffffffff}

// r2 = 2^512 mod p
var r2 = &Fq{0xfc9678ff0000000f, 0x67bb433d891a16e3, 0x7fae231004ccf590, 0x096d41af7ccfdaa9}

// r3 = 2^768 mod p
var r3 = &Fq{0x008b421c249dae4c, 0xe13bda50dba41326, 0x88fececb8e15cb63, 0x07dd97a06e6792c8}

// generator = 5 mod p is a generator of the `p - 1` order multiplicative
// subgroup, or in other words a primitive element of the field.
var generator = &Fq{0x96bc8c8cffffffed, 0x74c2a54b49f7778e, 0xfffffffffffffffd, 0x3fffffffffffffff}

var s = 32

// modulus representation
// p = 0x40000000000000000000000000000000224698fc0994a8dd8c46eb2100000001
var modulus = &Fq{0x8c46eb2100000001, 0x224698fc0994a8dd, 0x0000000000000000, 0x4000000000000000}

var BiModulus = new(big.Int).SetBytes([]byte{
	0x40, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x22, 0x46, 0x98, 0xfc, 0x09, 0x94, 0xa8, 0xdd,
	0x8c, 0x46, 0xeb, 0x21, 0x00, 0x00, 0x00, 0x01,
})

// Cmp returns -1 if fp < rhs
// 0 if fp == rhs
// 1 if fp > rhs
func (fq *Fq) Cmp(rhs *Fq) int {
	gt := 0
	lt := 0
	for i := len(fq) - 1; i >= 0; i-- {
		gt |= int((rhs[i]-fq[i])>>63) &^ lt
		lt |= int((fq[i]-rhs[i])>>63) &^ gt
	}
	return gt - lt
}

// Equal returns true if fp == rhs
func (fq *Fq) Equal(rhs *Fq) bool {
	t := fq[0] ^ rhs[0]
	t |= fq[1] ^ rhs[1]
	t |= fq[2] ^ rhs[2]
	t |= fq[3] ^ rhs[3]
	return t == 0
}

// IsZero returns true if fp == 0
func (fq *Fq) IsZero() bool {
	t := fq[0]
	t |= fq[1]
	t |= fq[2]
	t |= fq[3]
	return t == 0
}

// IsOne returns true if fp == r
func (fq *Fq) IsOne() bool {
	return fq.Equal(r)
}

// Set fp == rhs
func (fq *Fq) Set(rhs *Fq) *Fq {
	fq[0] = rhs[0]
	fq[1] = rhs[1]
	fq[2] = rhs[2]
	fq[3] = rhs[3]
	return fq
}

// SetUint64 sets fp == rhs
func (fq *Fq) SetUint64(rhs uint64) *Fq {
	r := &fiat_pasta_fq_non_montgomery_domain_field_element{rhs, 0, 0, 0}
	fiat_pasta_fq_to_montgomery((*fiat_pasta_fq_montgomery_domain_field_element)(fq), r)
	return fq
}

func (fq *Fq) SetBool(rhs bool) *Fq {
	if rhs {
		fq.SetOne()
	} else {
		fq.SetZero()
	}
	return fq
}

// SetOne fp == r
func (fq *Fq) SetOne() *Fq {
	return fq.Set(r)
}

// SetZero fp == 0
func (fq *Fq) SetZero() *Fq {
	fq[0] = 0
	fq[1] = 0
	fq[2] = 0
	fq[3] = 0
	return fq
}

// SetBytesWide takes 64 bytes as input and treats them as a 512-bit number.
// Attributed to https://github.com/zcash/pasta_curves/blob/main/src/fields/fq.rs#L255
// We reduce an arbitrary 512-bit number by decomposing it into two 256-bit digits
// with the higher bits multiplied by 2^256. Thus, we perform two reductions
//
// 1. the lower bits are multiplied by r^2, as normal
// 2. the upper bits are multiplied by r^2 * 2^256 = r^3
//
// and computing their sum in the field. It remains to see that arbitrary 256-bit
// numbers can be placed into Montgomery form safely using the reduction. The
// reduction works so long as the product is less than r=2^256 multiplied by
// the modulus. This holds because for any `c` smaller than the modulus, we have
// that (2^256 - 1)*c is an acceptable product for the reduction. Therefore, the
// reduction always works so long as `c` is in the field; in this case it is either the
// constant `r2` or `r3`.
func (fq *Fq) SetBytesWide(input *[64]byte) *Fq {
	d0 := fiat_pasta_fq_montgomery_domain_field_element{
		binary.LittleEndian.Uint64(input[:8]),
		binary.LittleEndian.Uint64(input[8:16]),
		binary.LittleEndian.Uint64(input[16:24]),
		binary.LittleEndian.Uint64(input[24:32]),
	}
	d1 := fiat_pasta_fq_montgomery_domain_field_element{
		binary.LittleEndian.Uint64(input[32:40]),
		binary.LittleEndian.Uint64(input[40:48]),
		binary.LittleEndian.Uint64(input[48:56]),
		binary.LittleEndian.Uint64(input[56:64]),
	}
	// Convert to Montgomery form
	tv1 := &fiat_pasta_fq_montgomery_domain_field_element{}
	tv2 := &fiat_pasta_fq_montgomery_domain_field_element{}
	// d0 * r2 + d1 * r3
	fiat_pasta_fq_mul(tv1, &d0, (*fiat_pasta_fq_montgomery_domain_field_element)(r2))
	fiat_pasta_fq_mul(tv2, &d1, (*fiat_pasta_fq_montgomery_domain_field_element)(r3))
	fiat_pasta_fq_add((*fiat_pasta_fq_montgomery_domain_field_element)(fq), tv1, tv2)
	return fq
}

// SetBytes attempts to convert a little endian byte representation
// of a scalar into a `Fq`, failing if input is not canonical
func (fq *Fq) SetBytes(input *[32]byte) (*Fq, error) {
	d0 := &Fq{
		binary.LittleEndian.Uint64(input[:8]),
		binary.LittleEndian.Uint64(input[8:16]),
		binary.LittleEndian.Uint64(input[16:24]),
		binary.LittleEndian.Uint64(input[24:32]),
	}
	if d0.Cmp(modulus) != -1 {
		return nil, fmt.Errorf("invalid byte sequence")
	}
	fiat_pasta_fq_from_bytes((*[4]uint64)(fq), input)
	fiat_pasta_fq_to_montgomery((*fiat_pasta_fq_montgomery_domain_field_element)(fq), (*fiat_pasta_fq_non_montgomery_domain_field_element)(fq))
	return fq, nil
}

// SetBigInt initializes an element from big.Int
// The value is reduced by the modulus
func (fq *Fq) SetBigInt(bi *big.Int) *Fq {
	var buffer [32]byte
	r := new(big.Int).Set(bi)
	r.Mod(r, BiModulus)
	r.FillBytes(buffer[:])
	copy(buffer[:], internal.ReverseScalarBytes(buffer[:]))
	_, _ = fq.SetBytes(&buffer)
	return fq
}

// SetRaw converts a raw array into a field element
func (fq *Fq) SetRaw(array *[4]uint64) *Fq {
	fiat_pasta_fq_to_montgomery((*fiat_pasta_fq_montgomery_domain_field_element)(fq), (*fiat_pasta_fq_non_montgomery_domain_field_element)(array))
	return fq
}

// Bytes converts this element into a byte representation
// in little endian byte order
func (fq *Fq) Bytes() [32]byte {
	var output [32]byte
	tv := &fiat_pasta_fq_non_montgomery_domain_field_element{}
	fiat_pasta_fq_from_montgomery(tv, (*fiat_pasta_fq_montgomery_domain_field_element)(fq))
	fiat_pasta_fq_to_bytes(&output, (*[4]uint64)(tv))
	return output
}

// BigInt converts this element into the big.Int struct
func (fq *Fq) BigInt() *big.Int {
	buffer := fq.Bytes()
	return new(big.Int).SetBytes(internal.ReverseScalarBytes(buffer[:]))
}

// Double this element
func (fq *Fq) Double(elem *Fq) *Fq {
	delem := (*fiat_pasta_fq_montgomery_domain_field_element)(elem)
	fiat_pasta_fq_add((*fiat_pasta_fq_montgomery_domain_field_element)(fq), delem, delem)
	return fq
}

// Square this element
func (fq *Fq) Square(elem *Fq) *Fq {
	delem := (*fiat_pasta_fq_montgomery_domain_field_element)(elem)
	fiat_pasta_fq_square((*fiat_pasta_fq_montgomery_domain_field_element)(fq), delem)
	return fq
}

// Sqrt this element, if it exists. If true, then value
// is a square root. If false, value is a QNR
func (fq *Fq) Sqrt(elem *Fq) (*Fq, bool) {
	return fq.tonelliShanks(elem)
}

// See sqrt_ts_ct at
// https://datatracker.ietf.org/doc/html/draft-irtf-cfrg-hash-to-curve-11#appendix-I.4
func (fq *Fq) tonelliShanks(elem *Fq) (*Fq, bool) {
	// c1 := 32
	// c2 := (q - 1) / (2^c1)
	//c2 := [4]uint64{
	//	0x0994a8dd8c46eb21,
	//	0x00000000224698fc,
	//	0x0000000000000000,
	//	0x0000000040000000,
	//}
	// c3 := (c2 - 1) / 2
	c3 := [4]uint64{
		0x04ca546ec6237590,
		0x0000000011234c7e,
		0x0000000000000000,
		0x0000000020000000,
	}
	//c4 := generator
	//c5 := new(Fq).pow(&generator, c2)
	c5 := &Fq{
		0x218077428c9942de,
		0xcc49578921b60494,
		0xac2e5d27b2efbee2,
		0xb79fa897f2db056,
	}

	z := new(Fq).pow(elem, c3)
	t := new(Fq).Square(z)
	t.Mul(t, elem)

	z.Mul(z, elem)

	b := new(Fq).Set(t)
	c := new(Fq).Set(c5)
	flags := map[bool]int{
		true:  1,
		false: 0,
	}

	for i := s; i >= 2; i-- {
		for j := 1; j <= i-2; j++ {
			b.Square(b)
		}
		z.CMove(z, new(Fq).Mul(z, c), flags[!b.IsOne()])
		c.Square(c)
		t.CMove(t, new(Fq).Mul(t, c), flags[!b.IsOne()])
		b.Set(t)
	}
	wasSquare := c.Square(z).Equal(elem)
	return fq.Set(z), wasSquare
}

// Invert this element i.e. compute the multiplicative inverse
// return false, zero if this element is zero
func (fq *Fq) Invert(elem *Fq) (*Fq, bool) {
	// computes elem^(p - 2) mod p
	exp := [4]uint64{
		0x8c46eb20ffffffff,
		0x224698fc0994a8dd,
		0x0000000000000000,
		0x4000000000000000}
	return fq.pow(elem, exp), !elem.IsZero()
}

// Mul returns the result from multiplying this element by rhs
func (fq *Fq) Mul(lhs, rhs *Fq) *Fq {
	dlhs := (*fiat_pasta_fq_montgomery_domain_field_element)(lhs)
	drhs := (*fiat_pasta_fq_montgomery_domain_field_element)(rhs)
	fiat_pasta_fq_mul((*fiat_pasta_fq_montgomery_domain_field_element)(fq), dlhs, drhs)
	return fq
}

// Sub returns the result from subtracting rhs from this element
func (fq *Fq) Sub(lhs, rhs *Fq) *Fq {
	dlhs := (*fiat_pasta_fq_montgomery_domain_field_element)(lhs)
	drhs := (*fiat_pasta_fq_montgomery_domain_field_element)(rhs)
	fiat_pasta_fq_sub((*fiat_pasta_fq_montgomery_domain_field_element)(fq), dlhs, drhs)
	return fq
}

// Add returns the result from adding rhs to this element
func (fq *Fq) Add(lhs, rhs *Fq) *Fq {
	dlhs := (*fiat_pasta_fq_montgomery_domain_field_element)(lhs)
	drhs := (*fiat_pasta_fq_montgomery_domain_field_element)(rhs)
	fiat_pasta_fq_add((*fiat_pasta_fq_montgomery_domain_field_element)(fq), dlhs, drhs)
	return fq
}

// Neg returns negation of this element
func (fq *Fq) Neg(elem *Fq) *Fq {
	delem := (*fiat_pasta_fq_montgomery_domain_field_element)(elem)
	fiat_pasta_fq_opp((*fiat_pasta_fq_montgomery_domain_field_element)(fq), delem)
	return fq
}

// Exp exponentiates this element by exp
func (fq *Fq) Exp(base, exp *Fq) *Fq {
	// convert exponent to integer form
	tv := &fiat_pasta_fq_non_montgomery_domain_field_element{}
	fiat_pasta_fq_from_montgomery(tv, (*fiat_pasta_fq_montgomery_domain_field_element)(exp))

	e := (*[4]uint64)(tv)
	return fq.pow(base, *e)
}

func (fq *Fq) pow(base *Fq, exp [4]uint64) *Fq {
	res := new(Fq).SetOne()
	tmp := new(Fq)

	for i := len(exp) - 1; i >= 0; i-- {
		for j := 63; j >= 0; j-- {
			res.Square(res)
			tmp.Mul(res, base)
			res.CMove(res, tmp, int(exp[i]>>j)&1)
		}
	}
	return fq.Set(res)
}

// CMove selects lhs if choice == 0 and rhs if choice == 1
func (fq *Fq) CMove(lhs, rhs *Fq, choice int) *Fq {
	dlhs := (*[4]uint64)(lhs)
	drhs := (*[4]uint64)(rhs)
	fiat_pasta_fq_selectznz((*[4]uint64)(fq), fiat_pasta_fq_uint1(choice), dlhs, drhs)
	return fq
}

// ToRaw converts this element into the a [4]uint64
func (fq *Fq) ToRaw() [4]uint64 {
	res := &fiat_pasta_fq_non_montgomery_domain_field_element{}
	fiat_pasta_fq_from_montgomery(res, (*fiat_pasta_fq_montgomery_domain_field_element)(fq))
	return *(*[4]uint64)(res)
}
