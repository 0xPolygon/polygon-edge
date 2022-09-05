package bls12381

import (
	"encoding/binary"
	"math/big"
	"sync"

	"github.com/coinbase/kryptology/pkg/core/curves/native"
)

type Fq [native.FieldLimbs]uint64

var bls12381FqInitonce sync.Once
var bls12381FqParams native.FieldParams

// 2^S * t = MODULUS - 1 with t odd
const fqS = 32

// qInv = -(q^{-1} mod 2^64) mod 2^64
const qInv = 0xfffffffeffffffff

// fqGenerator = 7 (multiplicative fqGenerator of r-1 order, that is also quadratic nonresidue)
var fqGenerator = [native.FieldLimbs]uint64{0x0000000efffffff1, 0x17e363d300189c0f, 0xff9c57876f8457b0, 0x351332208fc5a8c4}

// fqModulus
var fqModulus = [native.FieldLimbs]uint64{0xffffffff00000001, 0x53bda402fffe5bfe, 0x3339d80809a1d805, 0x73eda753299d7d48}

func Bls12381FqNew() *native.Field {
	return &native.Field{
		Value:      [native.FieldLimbs]uint64{},
		Params:     getBls12381FqParams(),
		Arithmetic: bls12381FqArithmetic{},
	}
}

func bls12381FqParamsInit() {
	bls12381FqParams = native.FieldParams{
		R:       [native.FieldLimbs]uint64{0x00000001fffffffe, 0x5884b7fa00034802, 0x998c4fefecbc4ff5, 0x1824b159acc5056f},
		R2:      [native.FieldLimbs]uint64{0xc999e990f3f29c6d, 0x2b6cedcb87925c23, 0x05d314967254398f, 0x0748d9d99f59ff11},
		R3:      [native.FieldLimbs]uint64{0xc62c1807439b73af, 0x1b3e0d188cf06990, 0x73d13c71c7b5f418, 0x6e2a5bb9c8db33e9},
		Modulus: [native.FieldLimbs]uint64{0xffffffff00000001, 0x53bda402fffe5bfe, 0x3339d80809a1d805, 0x73eda753299d7d48},
		BiModulus: new(big.Int).SetBytes([]byte{
			0x73, 0xed, 0xa7, 0x53, 0x29, 0x9d, 0x7d, 0x48, 0x33, 0x39, 0xd8, 0x08, 0x09, 0xa1, 0xd8, 0x05, 0x53, 0xbd, 0xa4, 0x02, 0xff, 0xfe, 0x5b, 0xfe, 0xff, 0xff, 0xff, 0xff, 0x00, 0x00, 0x00, 0x01}),
	}
}

func getBls12381FqParams() *native.FieldParams {
	bls12381FqInitonce.Do(bls12381FqParamsInit)
	return &bls12381FqParams
}

// bls12381FqArithmetic is a struct with all the methods needed for working
// in mod q
type bls12381FqArithmetic struct{}

// ToMontgomery converts this field to montgomery form
func (f bls12381FqArithmetic) ToMontgomery(out, arg *[native.FieldLimbs]uint64) {
	// arg.R^0 * R^2 / R = arg.R
	f.Mul(out, arg, &getBls12381FqParams().R2)
}

// FromMontgomery converts this field from montgomery form
func (f bls12381FqArithmetic) FromMontgomery(out, arg *[native.FieldLimbs]uint64) {
	// Mul by 1 is division by 2^256 mod q
	//f.Mul(out, arg, &[native.FieldLimbs]uint64{1, 0, 0, 0})
	f.montReduce(out, &[native.FieldLimbs * 2]uint64{arg[0], arg[1], arg[2], arg[3], 0, 0, 0, 0})
}

// Neg performs modular negation
func (f bls12381FqArithmetic) Neg(out, arg *[native.FieldLimbs]uint64) {
	// Subtract `arg` from `fqModulus`. Ignore final borrow
	// since it can't underflow.
	var t [native.FieldLimbs]uint64
	var borrow uint64
	t[0], borrow = sbb(fqModulus[0], arg[0], 0)
	t[1], borrow = sbb(fqModulus[1], arg[1], borrow)
	t[2], borrow = sbb(fqModulus[2], arg[2], borrow)
	t[3], _ = sbb(fqModulus[3], arg[3], borrow)

	// t could be `fqModulus` if `arg`=0. Set mask=0 if self=0
	// and 0xff..ff if `arg`!=0
	mask := t[0] | t[1] | t[2] | t[3]
	mask = -((mask | -mask) >> 63)
	out[0] = t[0] & mask
	out[1] = t[1] & mask
	out[2] = t[2] & mask
	out[3] = t[3] & mask
}

// Square performs modular square
func (f bls12381FqArithmetic) Square(out, arg *[native.FieldLimbs]uint64) {
	var r [2 * native.FieldLimbs]uint64
	var carry uint64

	r[1], carry = mac(0, arg[0], arg[1], 0)
	r[2], carry = mac(0, arg[0], arg[2], carry)
	r[3], r[4] = mac(0, arg[0], arg[3], carry)

	r[3], carry = mac(r[3], arg[1], arg[2], 0)
	r[4], r[5] = mac(r[4], arg[1], arg[3], carry)

	r[5], r[6] = mac(r[5], arg[2], arg[3], 0)

	r[7] = r[6] >> 63
	r[6] = (r[6] << 1) | r[5]>>63
	r[5] = (r[5] << 1) | r[4]>>63
	r[4] = (r[4] << 1) | r[3]>>63
	r[3] = (r[3] << 1) | r[2]>>63
	r[2] = (r[2] << 1) | r[1]>>63
	r[1] = r[1] << 1

	r[0], carry = mac(0, arg[0], arg[0], 0)
	r[1], carry = adc(0, r[1], carry)
	r[2], carry = mac(r[2], arg[1], arg[1], carry)
	r[3], carry = adc(0, r[3], carry)
	r[4], carry = mac(r[4], arg[2], arg[2], carry)
	r[5], carry = adc(0, r[5], carry)
	r[6], carry = mac(r[6], arg[3], arg[3], carry)
	r[7], _ = adc(0, r[7], carry)

	f.montReduce(out, &r)
}

// Mul performs modular multiplication
func (f bls12381FqArithmetic) Mul(out, arg1, arg2 *[native.FieldLimbs]uint64) {
	// Schoolbook multiplication
	var r [2 * native.FieldLimbs]uint64
	var carry uint64

	r[0], carry = mac(0, arg1[0], arg2[0], 0)
	r[1], carry = mac(0, arg1[0], arg2[1], carry)
	r[2], carry = mac(0, arg1[0], arg2[2], carry)
	r[3], r[4] = mac(0, arg1[0], arg2[3], carry)

	r[1], carry = mac(r[1], arg1[1], arg2[0], 0)
	r[2], carry = mac(r[2], arg1[1], arg2[1], carry)
	r[3], carry = mac(r[3], arg1[1], arg2[2], carry)
	r[4], r[5] = mac(r[4], arg1[1], arg2[3], carry)

	r[2], carry = mac(r[2], arg1[2], arg2[0], 0)
	r[3], carry = mac(r[3], arg1[2], arg2[1], carry)
	r[4], carry = mac(r[4], arg1[2], arg2[2], carry)
	r[5], r[6] = mac(r[5], arg1[2], arg2[3], carry)

	r[3], carry = mac(r[3], arg1[3], arg2[0], 0)
	r[4], carry = mac(r[4], arg1[3], arg2[1], carry)
	r[5], carry = mac(r[5], arg1[3], arg2[2], carry)
	r[6], r[7] = mac(r[6], arg1[3], arg2[3], carry)

	f.montReduce(out, &r)
}

// Add performs modular addition
func (f bls12381FqArithmetic) Add(out, arg1, arg2 *[native.FieldLimbs]uint64) {
	var t [native.FieldLimbs]uint64
	var carry uint64

	t[0], carry = adc(arg1[0], arg2[0], 0)
	t[1], carry = adc(arg1[1], arg2[1], carry)
	t[2], carry = adc(arg1[2], arg2[2], carry)
	t[3], _ = adc(arg1[3], arg2[3], carry)

	// Subtract the fqModulus to ensure the value
	// is smaller.
	f.Sub(out, &t, &fqModulus)
}

// Sub performs modular subtraction
func (f bls12381FqArithmetic) Sub(out, arg1, arg2 *[native.FieldLimbs]uint64) {
	d0, borrow := sbb(arg1[0], arg2[0], 0)
	d1, borrow := sbb(arg1[1], arg2[1], borrow)
	d2, borrow := sbb(arg1[2], arg2[2], borrow)
	d3, borrow := sbb(arg1[3], arg2[3], borrow)

	// If underflow occurred on the final limb, borrow 0xff...ff, otherwise
	// borrow = 0x00...00. Conditionally mask to add the fqModulus
	borrow = -borrow
	d0, carry := adc(d0, fqModulus[0]&borrow, 0)
	d1, carry = adc(d1, fqModulus[1]&borrow, carry)
	d2, carry = adc(d2, fqModulus[2]&borrow, carry)
	d3, _ = adc(d3, fqModulus[3]&borrow, carry)

	out[0] = d0
	out[1] = d1
	out[2] = d2
	out[3] = d3
}

// Sqrt performs modular square root
func (f bls12381FqArithmetic) Sqrt(wasSquare *int, out, arg *[native.FieldLimbs]uint64) {
	// See sqrt_ts_ct at
	// https://datatracker.ietf.org/doc/html/draft-irtf-cfrg-hash-to-curve-11#appendix-I.4
	// c1 := fqS
	// c2 := (q - 1) / (2^c1)
	c2 := [4]uint64{
		0xfffe5bfeffffffff,
		0x09a1d80553bda402,
		0x299d7d483339d808,
		0x0000000073eda753,
	}
	// c3 := (c2 - 1) / 2
	c3 := [native.FieldLimbs]uint64{
		0x7fff2dff7fffffff,
		0x04d0ec02a9ded201,
		0x94cebea4199cec04,
		0x0000000039f6d3a9,
	}
	//c4 := fqGenerator
	var c5 [native.FieldLimbs]uint64
	native.Pow(&c5, &fqGenerator, &c2, getBls12381FqParams(), f)
	//c5 := [native.FieldLimbs]uint64{0x1015708f7e368fe1, 0x31c6c5456ecc4511, 0x5281fe8998a19ea1, 0x0279089e10c63fe8}
	var z, t, b, c, tv [native.FieldLimbs]uint64

	native.Pow(&z, arg, &c3, getBls12381FqParams(), f)
	f.Square(&t, &z)
	f.Mul(&t, &t, arg)
	f.Mul(&z, &z, arg)

	copy(b[:], t[:])
	copy(c[:], c5[:])

	for i := fqS; i >= 2; i-- {
		for j := 1; j <= i-2; j++ {
			f.Square(&b, &b)
		}
		// if b == 1 flag = 0 else flag = 1
		flag := -(&native.Field{
			Value:      b,
			Params:     getBls12381FqParams(),
			Arithmetic: f,
		}).IsOne() + 1
		f.Mul(&tv, &z, &c)
		f.Selectznz(&z, &z, &tv, flag)
		f.Square(&c, &c)
		f.Mul(&tv, &t, &c)
		f.Selectznz(&t, &t, &tv, flag)
		copy(b[:], t[:])
	}
	f.Square(&c, &z)
	*wasSquare = (&native.Field{
		Value:      c,
		Params:     getBls12381FqParams(),
		Arithmetic: f,
	}).Equal(&native.Field{
		Value:      *arg,
		Params:     getBls12381FqParams(),
		Arithmetic: f,
	})
	f.Selectznz(out, out, &z, *wasSquare)
}

// Invert performs modular inverse
func (f bls12381FqArithmetic) Invert(wasInverted *int, out, arg *[native.FieldLimbs]uint64) {
	// Using an addition chain from
	// https://github.com/kwantam/addchain
	var t0, t1, t2, t3, t4, t5, t6, t7, t8 [native.FieldLimbs]uint64
	var t9, t11, t12, t13, t14, t15, t16, t17 [native.FieldLimbs]uint64

	f.Square(&t0, arg)
	f.Mul(&t1, &t0, arg)
	f.Square(&t16, &t0)
	f.Square(&t6, &t16)
	f.Mul(&t5, &t6, &t0)
	f.Mul(&t0, &t6, &t16)
	f.Mul(&t12, &t5, &t16)
	f.Square(&t2, &t6)
	f.Mul(&t7, &t5, &t6)
	f.Mul(&t15, &t0, &t5)
	f.Square(&t17, &t12)
	f.Mul(&t1, &t1, &t17)
	f.Mul(&t3, &t7, &t2)
	f.Mul(&t8, &t1, &t17)
	f.Mul(&t4, &t8, &t2)
	f.Mul(&t9, &t8, &t7)
	f.Mul(&t7, &t4, &t5)
	f.Mul(&t11, &t4, &t17)
	f.Mul(&t5, &t9, &t17)
	f.Mul(&t14, &t7, &t15)
	f.Mul(&t13, &t11, &t12)
	f.Mul(&t12, &t11, &t17)
	f.Mul(&t15, &t15, &t12)
	f.Mul(&t16, &t16, &t15)
	f.Mul(&t3, &t3, &t16)
	f.Mul(&t17, &t17, &t3)
	f.Mul(&t0, &t0, &t17)
	f.Mul(&t6, &t6, &t0)
	f.Mul(&t2, &t2, &t6)
	native.Pow2k(&t0, &t0, 8, f)
	f.Mul(&t0, &t0, &t17)
	native.Pow2k(&t0, &t0, 9, f)
	f.Mul(&t0, &t0, &t16)
	native.Pow2k(&t0, &t0, 9, f)
	f.Mul(&t0, &t0, &t15)
	native.Pow2k(&t0, &t0, 9, f)
	f.Mul(&t0, &t0, &t15)
	native.Pow2k(&t0, &t0, 7, f)
	f.Mul(&t0, &t0, &t14)
	native.Pow2k(&t0, &t0, 7, f)
	f.Mul(&t0, &t0, &t13)
	native.Pow2k(&t0, &t0, 10, f)
	f.Mul(&t0, &t0, &t12)
	native.Pow2k(&t0, &t0, 9, f)
	f.Mul(&t0, &t0, &t11)
	native.Pow2k(&t0, &t0, 8, f)
	f.Mul(&t0, &t0, &t8)
	native.Pow2k(&t0, &t0, 8, f)
	f.Mul(&t0, &t0, arg)
	native.Pow2k(&t0, &t0, 14, f)
	f.Mul(&t0, &t0, &t9)
	native.Pow2k(&t0, &t0, 10, f)
	f.Mul(&t0, &t0, &t8)
	native.Pow2k(&t0, &t0, 15, f)
	f.Mul(&t0, &t0, &t7)
	native.Pow2k(&t0, &t0, 10, f)
	f.Mul(&t0, &t0, &t6)
	native.Pow2k(&t0, &t0, 8, f)
	f.Mul(&t0, &t0, &t5)
	native.Pow2k(&t0, &t0, 16, f)
	f.Mul(&t0, &t0, &t3)
	native.Pow2k(&t0, &t0, 8, f)
	f.Mul(&t0, &t0, &t2)
	native.Pow2k(&t0, &t0, 7, f)
	f.Mul(&t0, &t0, &t4)
	native.Pow2k(&t0, &t0, 9, f)
	f.Mul(&t0, &t0, &t2)
	native.Pow2k(&t0, &t0, 8, f)
	f.Mul(&t0, &t0, &t3)
	native.Pow2k(&t0, &t0, 8, f)
	f.Mul(&t0, &t0, &t2)
	native.Pow2k(&t0, &t0, 8, f)
	f.Mul(&t0, &t0, &t2)
	native.Pow2k(&t0, &t0, 8, f)
	f.Mul(&t0, &t0, &t2)
	native.Pow2k(&t0, &t0, 8, f)
	f.Mul(&t0, &t0, &t3)
	native.Pow2k(&t0, &t0, 8, f)
	f.Mul(&t0, &t0, &t2)
	native.Pow2k(&t0, &t0, 8, f)
	f.Mul(&t0, &t0, &t2)
	native.Pow2k(&t0, &t0, 5, f)
	f.Mul(&t0, &t0, &t1)
	native.Pow2k(&t0, &t0, 5, f)
	f.Mul(&t0, &t0, &t1)

	*wasInverted = (&native.Field{
		Value:      *arg,
		Params:     getBls12381FqParams(),
		Arithmetic: f,
	}).IsNonZero()
	f.Selectznz(out, out, &t0, *wasInverted)
}

// FromBytes converts a little endian byte array into a field element
func (f bls12381FqArithmetic) FromBytes(out *[native.FieldLimbs]uint64, arg *[native.FieldBytes]byte) {
	out[0] = binary.LittleEndian.Uint64(arg[:8])
	out[1] = binary.LittleEndian.Uint64(arg[8:16])
	out[2] = binary.LittleEndian.Uint64(arg[16:24])
	out[3] = binary.LittleEndian.Uint64(arg[24:])
}

// ToBytes converts a field element to a little endian byte array
func (f bls12381FqArithmetic) ToBytes(out *[native.FieldBytes]byte, arg *[native.FieldLimbs]uint64) {
	binary.LittleEndian.PutUint64(out[:8], arg[0])
	binary.LittleEndian.PutUint64(out[8:16], arg[1])
	binary.LittleEndian.PutUint64(out[16:24], arg[2])
	binary.LittleEndian.PutUint64(out[24:], arg[3])
}

// Selectznz performs conditional select.
// selects arg1 if choice == 0 and arg2 if choice == 1
func (f bls12381FqArithmetic) Selectznz(out, arg1, arg2 *[native.FieldLimbs]uint64, choice int) {
	b := uint64(-choice)
	out[0] = arg1[0] ^ ((arg1[0] ^ arg2[0]) & b)
	out[1] = arg1[1] ^ ((arg1[1] ^ arg2[1]) & b)
	out[2] = arg1[2] ^ ((arg1[2] ^ arg2[2]) & b)
	out[3] = arg1[3] ^ ((arg1[3] ^ arg2[3]) & b)
}

func (f bls12381FqArithmetic) montReduce(out *[native.FieldLimbs]uint64, r *[2 * native.FieldLimbs]uint64) {
	// Taken from Algorithm 14.32 in Handbook of Applied Cryptography
	var r1, r2, r3, r4, r5, r6, carry, carry2, k uint64
	var rr [native.FieldLimbs]uint64

	k = r[0] * qInv
	_, carry = mac(r[0], k, fqModulus[0], 0)
	r1, carry = mac(r[1], k, fqModulus[1], carry)
	r2, carry = mac(r[2], k, fqModulus[2], carry)
	r3, carry = mac(r[3], k, fqModulus[3], carry)
	r4, carry2 = adc(r[4], 0, carry)

	k = r1 * qInv
	_, carry = mac(r1, k, fqModulus[0], 0)
	r2, carry = mac(r2, k, fqModulus[1], carry)
	r3, carry = mac(r3, k, fqModulus[2], carry)
	r4, carry = mac(r4, k, fqModulus[3], carry)
	r5, carry2 = adc(r[5], carry2, carry)

	k = r2 * qInv
	_, carry = mac(r2, k, fqModulus[0], 0)
	r3, carry = mac(r3, k, fqModulus[1], carry)
	r4, carry = mac(r4, k, fqModulus[2], carry)
	r5, carry = mac(r5, k, fqModulus[3], carry)
	r6, carry2 = adc(r[6], carry2, carry)

	k = r3 * qInv
	_, carry = mac(r3, k, fqModulus[0], 0)
	rr[0], carry = mac(r4, k, fqModulus[1], carry)
	rr[1], carry = mac(r5, k, fqModulus[2], carry)
	rr[2], carry = mac(r6, k, fqModulus[3], carry)
	rr[3], _ = adc(r[7], carry2, carry)

	f.Sub(out, &rr, &fqModulus)
}
