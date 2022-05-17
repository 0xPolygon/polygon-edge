package bls12381

import (
	"encoding/binary"
	"fmt"
	"io"
	"math/big"

	"github.com/coinbase/kryptology/internal"
	"github.com/coinbase/kryptology/pkg/core/curves/native"
)

// fp field element mod p
type fp [Limbs]uint64

var (
	modulus = fp{
		0xb9feffffffffaaab,
		0x1eabfffeb153ffff,
		0x6730d2a0f6b0f624,
		0x64774b84f38512bf,
		0x4b1ba7b6434bacd7,
		0x1a0111ea397fe69a,
	}
	halfModulus = fp{
		0xdcff_7fff_ffff_d556,
		0x0f55_ffff_58a9_ffff,
		0xb398_6950_7b58_7b12,
		0xb23b_a5c2_79c2_895f,
		0x258d_d3db_21a5_d66b,
		0x0d00_88f5_1cbf_f34d,
	}
	// 2^256 mod p
	r = fp{
		0x760900000002fffd,
		0xebf4000bc40c0002,
		0x5f48985753c758ba,
		0x77ce585370525745,
		0x5c071a97a256ec6d,
		0x15f65ec3fa80e493,
	}
	// 2^512 mod p
	r2 = fp{
		0xf4df1f341c341746,
		0x0a76e6a609d104f1,
		0x8de5476c4c95b6d5,
		0x67eb88a9939d83c0,
		0x9a793e85b519952d,
		0x11988fe592cae3aa,
	}
	// 2^768 mod p
	r3 = fp{
		0xed48ac6bd94ca1e0,
		0x315f831e03a7adf8,
		0x9a53352a615e29dd,
		0x34c04e5e921e1761,
		0x2512d43565724728,
		0x0aa6346091755d4d,
	}
	biModulus = new(big.Int).SetBytes([]byte{
		0x1a, 0x01, 0x11, 0xea, 0x39, 0x7f, 0xe6, 0x9a, 0x4b, 0x1b, 0xa7, 0xb6, 0x43, 0x4b, 0xac, 0xd7, 0x64, 0x77, 0x4b, 0x84, 0xf3, 0x85, 0x12, 0xbf, 0x67, 0x30, 0xd2, 0xa0, 0xf6, 0xb0, 0xf6, 0x24, 0x1e, 0xab, 0xff, 0xfe, 0xb1, 0x53, 0xff, 0xff, 0xb9, 0xfe, 0xff, 0xff, 0xff, 0xff, 0xaa, 0xab},
	)
)

// inv = -(p^{-1} mod 2^64) mod 2^64
const inv = 0x89f3_fffc_fffc_fffd
const hashBytes = 64

// IsZero returns 1 if fp == 0, 0 otherwise
func (f *fp) IsZero() int {
	t := f[0]
	t |= f[1]
	t |= f[2]
	t |= f[3]
	t |= f[4]
	t |= f[5]
	return int(((int64(t) | int64(-t)) >> 63) + 1)
}

// IsNonZero returns 1 if fp != 0, 0 otherwise
func (f *fp) IsNonZero() int {
	t := f[0]
	t |= f[1]
	t |= f[2]
	t |= f[3]
	t |= f[4]
	t |= f[5]
	return int(-((int64(t) | int64(-t)) >> 63))
}

// IsOne returns 1 if fp == 1, 0 otherwise
func (f *fp) IsOne() int {
	return f.Equal(&r)
}

// Cmp returns -1 if f < rhs
// 0 if f == rhs
// 1 if f > rhs
func (f *fp) Cmp(rhs *fp) int {
	gt := uint64(0)
	lt := uint64(0)
	for i := 5; i >= 0; i-- {
		// convert to two 64-bit numbers where
		// the leading bits are zeros and hold no meaning
		//  so rhs - f actually means gt
		// and f - rhs actually means lt.
		rhsH := rhs[i] >> 32
		rhsL := rhs[i] & 0xffffffff
		lhsH := f[i] >> 32
		lhsL := f[i] & 0xffffffff

		// Check the leading bit
		// if negative then f > rhs
		// if positive then f < rhs
		gt |= (rhsH - lhsH) >> 32 & 1 &^ lt
		lt |= (lhsH - rhsH) >> 32 & 1 &^ gt
		gt |= (rhsL - lhsL) >> 32 & 1 &^ lt
		lt |= (lhsL - rhsL) >> 32 & 1 &^ gt
	}
	// Make the result -1 for <, 0 for =, 1 for >
	return int(gt) - int(lt)
}

// Equal returns 1 if fp == rhs, 0 otherwise
func (f *fp) Equal(rhs *fp) int {
	t := f[0] ^ rhs[0]
	t |= f[1] ^ rhs[1]
	t |= f[2] ^ rhs[2]
	t |= f[3] ^ rhs[3]
	t |= f[4] ^ rhs[4]
	t |= f[5] ^ rhs[5]
	return int(((int64(t) | int64(-t)) >> 63) + 1)
}

// LexicographicallyLargest returns 1 if
// this element is strictly lexicographically larger than its negation
// 0 otherwise
func (f *fp) LexicographicallyLargest() int {
	var ff fp
	ff.fromMontgomery(f)

	_, borrow := sbb(ff[0], halfModulus[0], 0)
	_, borrow = sbb(ff[1], halfModulus[1], borrow)
	_, borrow = sbb(ff[2], halfModulus[2], borrow)
	_, borrow = sbb(ff[3], halfModulus[3], borrow)
	_, borrow = sbb(ff[4], halfModulus[4], borrow)
	_, borrow = sbb(ff[5], halfModulus[5], borrow)

	return (int(borrow) - 1) & 1
}

// Sgn0 returns the lowest bit value
func (f *fp) Sgn0() int {
	t := new(fp).fromMontgomery(f)
	return int(t[0] & 1)
}

// SetOne fp = r
func (f *fp) SetOne() *fp {
	f[0] = r[0]
	f[1] = r[1]
	f[2] = r[2]
	f[3] = r[3]
	f[4] = r[4]
	f[5] = r[5]
	return f
}

// SetZero fp = 0
func (f *fp) SetZero() *fp {
	f[0] = 0
	f[1] = 0
	f[2] = 0
	f[3] = 0
	f[4] = 0
	f[5] = 0
	return f
}

// SetUint64 fp = rhs
func (f *fp) SetUint64(rhs uint64) *fp {
	f[0] = rhs
	f[1] = 0
	f[2] = 0
	f[3] = 0
	f[4] = 0
	f[5] = 0
	return f.toMontgomery(f)
}

// Random generates a random field element
func (f *fp) Random(reader io.Reader) (*fp, error) {
	var t [WideFieldBytes]byte
	n, err := reader.Read(t[:])
	if err != nil {
		return nil, err
	}
	if n != WideFieldBytes {
		return nil, fmt.Errorf("can only read %d when %d are needed", n, WideFieldBytes)
	}
	return f.Hash(t[:]), nil
}

// Hash converts the byte sequence into a field element
func (f *fp) Hash(input []byte) *fp {
	dst := []byte("BLS12381_XMD:SHA-256_SSWU_RO_")
	xmd := native.ExpandMsgXmd(native.EllipticPointHasherSha256(), input, dst, hashBytes)
	var t [WideFieldBytes]byte
	copy(t[:hashBytes], internal.ReverseScalarBytes(xmd))
	return f.SetBytesWide(&t)
}

// toMontgomery converts this field to montgomery form
func (f *fp) toMontgomery(a *fp) *fp {
	// arg.R^0 * R^2 / R = arg.R
	return f.Mul(a, &r2)
}

// fromMontgomery converts this field from montgomery form
func (f *fp) fromMontgomery(a *fp) *fp {
	// Mul by 1 is division by 2^256 mod q
	//out.Mul(arg, &[native.FieldLimbs]uint64{1, 0, 0, 0})
	return f.montReduce(&[Limbs * 2]uint64{a[0], a[1], a[2], a[3], a[4], a[5], 0, 0, 0, 0, 0, 0})
}

// Neg performs modular negation
func (f *fp) Neg(a *fp) *fp {
	// Subtract `arg` from `modulus`. Ignore final borrow
	// since it can't underflow.
	var t [Limbs]uint64
	var borrow uint64
	t[0], borrow = sbb(modulus[0], a[0], 0)
	t[1], borrow = sbb(modulus[1], a[1], borrow)
	t[2], borrow = sbb(modulus[2], a[2], borrow)
	t[3], borrow = sbb(modulus[3], a[3], borrow)
	t[4], borrow = sbb(modulus[4], a[4], borrow)
	t[5], _ = sbb(modulus[5], a[5], borrow)

	// t could be `modulus` if `arg`=0. Set mask=0 if self=0
	// and 0xff..ff if `arg`!=0
	mask := a[0] | a[1] | a[2] | a[3] | a[4] | a[5]
	mask = -((mask | -mask) >> 63)
	f[0] = t[0] & mask
	f[1] = t[1] & mask
	f[2] = t[2] & mask
	f[3] = t[3] & mask
	f[4] = t[4] & mask
	f[5] = t[5] & mask
	return f
}

// Square performs modular square
func (f *fp) Square(a *fp) *fp {
	var r [2 * Limbs]uint64
	var carry uint64

	r[1], carry = mac(0, a[0], a[1], 0)
	r[2], carry = mac(0, a[0], a[2], carry)
	r[3], carry = mac(0, a[0], a[3], carry)
	r[4], carry = mac(0, a[0], a[4], carry)
	r[5], r[6] = mac(0, a[0], a[5], carry)

	r[3], carry = mac(r[3], a[1], a[2], 0)
	r[4], carry = mac(r[4], a[1], a[3], carry)
	r[5], carry = mac(r[5], a[1], a[4], carry)
	r[6], r[7] = mac(r[6], a[1], a[5], carry)

	r[5], carry = mac(r[5], a[2], a[3], 0)
	r[6], carry = mac(r[6], a[2], a[4], carry)
	r[7], r[8] = mac(r[7], a[2], a[5], carry)

	r[7], carry = mac(r[7], a[3], a[4], 0)
	r[8], r[9] = mac(r[8], a[3], a[5], carry)

	r[9], r[10] = mac(r[9], a[4], a[5], 0)

	r[11] = r[10] >> 63
	r[10] = (r[10] << 1) | r[9]>>63
	r[9] = (r[9] << 1) | r[8]>>63
	r[8] = (r[8] << 1) | r[7]>>63
	r[7] = (r[7] << 1) | r[6]>>63
	r[6] = (r[6] << 1) | r[5]>>63
	r[5] = (r[5] << 1) | r[4]>>63
	r[4] = (r[4] << 1) | r[3]>>63
	r[3] = (r[3] << 1) | r[2]>>63
	r[2] = (r[2] << 1) | r[1]>>63
	r[1] = r[1] << 1

	r[0], carry = mac(0, a[0], a[0], 0)
	r[1], carry = adc(0, r[1], carry)
	r[2], carry = mac(r[2], a[1], a[1], carry)
	r[3], carry = adc(0, r[3], carry)
	r[4], carry = mac(r[4], a[2], a[2], carry)
	r[5], carry = adc(0, r[5], carry)
	r[6], carry = mac(r[6], a[3], a[3], carry)
	r[7], carry = adc(0, r[7], carry)
	r[8], carry = mac(r[8], a[4], a[4], carry)
	r[9], carry = adc(0, r[9], carry)
	r[10], carry = mac(r[10], a[5], a[5], carry)
	r[11], _ = adc(0, r[11], carry)

	return f.montReduce(&r)
}

// Double this element
func (f *fp) Double(a *fp) *fp {
	return f.Add(a, a)
}

// Mul performs modular multiplication
func (f *fp) Mul(arg1, arg2 *fp) *fp {
	// Schoolbook multiplication
	var r [2 * Limbs]uint64
	var carry uint64

	r[0], carry = mac(0, arg1[0], arg2[0], 0)
	r[1], carry = mac(0, arg1[0], arg2[1], carry)
	r[2], carry = mac(0, arg1[0], arg2[2], carry)
	r[3], carry = mac(0, arg1[0], arg2[3], carry)
	r[4], carry = mac(0, arg1[0], arg2[4], carry)
	r[5], r[6] = mac(0, arg1[0], arg2[5], carry)

	r[1], carry = mac(r[1], arg1[1], arg2[0], 0)
	r[2], carry = mac(r[2], arg1[1], arg2[1], carry)
	r[3], carry = mac(r[3], arg1[1], arg2[2], carry)
	r[4], carry = mac(r[4], arg1[1], arg2[3], carry)
	r[5], carry = mac(r[5], arg1[1], arg2[4], carry)
	r[6], r[7] = mac(r[6], arg1[1], arg2[5], carry)

	r[2], carry = mac(r[2], arg1[2], arg2[0], 0)
	r[3], carry = mac(r[3], arg1[2], arg2[1], carry)
	r[4], carry = mac(r[4], arg1[2], arg2[2], carry)
	r[5], carry = mac(r[5], arg1[2], arg2[3], carry)
	r[6], carry = mac(r[6], arg1[2], arg2[4], carry)
	r[7], r[8] = mac(r[7], arg1[2], arg2[5], carry)

	r[3], carry = mac(r[3], arg1[3], arg2[0], 0)
	r[4], carry = mac(r[4], arg1[3], arg2[1], carry)
	r[5], carry = mac(r[5], arg1[3], arg2[2], carry)
	r[6], carry = mac(r[6], arg1[3], arg2[3], carry)
	r[7], carry = mac(r[7], arg1[3], arg2[4], carry)
	r[8], r[9] = mac(r[8], arg1[3], arg2[5], carry)

	r[4], carry = mac(r[4], arg1[4], arg2[0], 0)
	r[5], carry = mac(r[5], arg1[4], arg2[1], carry)
	r[6], carry = mac(r[6], arg1[4], arg2[2], carry)
	r[7], carry = mac(r[7], arg1[4], arg2[3], carry)
	r[8], carry = mac(r[8], arg1[4], arg2[4], carry)
	r[9], r[10] = mac(r[9], arg1[4], arg2[5], carry)

	r[5], carry = mac(r[5], arg1[5], arg2[0], 0)
	r[6], carry = mac(r[6], arg1[5], arg2[1], carry)
	r[7], carry = mac(r[7], arg1[5], arg2[2], carry)
	r[8], carry = mac(r[8], arg1[5], arg2[3], carry)
	r[9], carry = mac(r[9], arg1[5], arg2[4], carry)
	r[10], r[11] = mac(r[10], arg1[5], arg2[5], carry)

	return f.montReduce(&r)
}

// MulBy3b returns arg * 12 or 3 * b
func (f *fp) MulBy3b(arg *fp) *fp {
	var a, t fp
	a.Double(arg) // 2
	t.Double(&a)  // 4
	a.Double(&t)  // 8
	a.Add(&a, &t) // 12
	return f.Set(&a)
}

// Add performs modular addition
func (f *fp) Add(arg1, arg2 *fp) *fp {
	var t fp
	var carry uint64

	t[0], carry = adc(arg1[0], arg2[0], 0)
	t[1], carry = adc(arg1[1], arg2[1], carry)
	t[2], carry = adc(arg1[2], arg2[2], carry)
	t[3], carry = adc(arg1[3], arg2[3], carry)
	t[4], carry = adc(arg1[4], arg2[4], carry)
	t[5], _ = adc(arg1[5], arg2[5], carry)

	// Subtract the modulus to ensure the value
	// is smaller.
	return f.Sub(&t, &modulus)
}

// Sub performs modular subtraction
func (f *fp) Sub(arg1, arg2 *fp) *fp {
	d0, borrow := sbb(arg1[0], arg2[0], 0)
	d1, borrow := sbb(arg1[1], arg2[1], borrow)
	d2, borrow := sbb(arg1[2], arg2[2], borrow)
	d3, borrow := sbb(arg1[3], arg2[3], borrow)
	d4, borrow := sbb(arg1[4], arg2[4], borrow)
	d5, borrow := sbb(arg1[5], arg2[5], borrow)

	// If underflow occurred on the final limb, borrow 0xff...ff, otherwise
	// borrow = 0x00...00. Conditionally mask to add the modulus
	borrow = -borrow
	d0, carry := adc(d0, modulus[0]&borrow, 0)
	d1, carry = adc(d1, modulus[1]&borrow, carry)
	d2, carry = adc(d2, modulus[2]&borrow, carry)
	d3, carry = adc(d3, modulus[3]&borrow, carry)
	d4, carry = adc(d4, modulus[4]&borrow, carry)
	d5, _ = adc(d5, modulus[5]&borrow, carry)

	f[0] = d0
	f[1] = d1
	f[2] = d2
	f[3] = d3
	f[4] = d4
	f[5] = d5
	return f
}

// Sqrt performs modular square root
func (f *fp) Sqrt(a *fp) (*fp, int) {
	// Shank's method, as p = 3 (mod 4). This means
	// exponentiate by (p+1)/4. This only works for elements
	// that are actually quadratic residue,
	// so check the result at the end.
	var c, z fp
	z.pow(a, &fp{
		0xee7fbfffffffeaab,
		0x07aaffffac54ffff,
		0xd9cc34a83dac3d89,
		0xd91dd2e13ce144af,
		0x92c6e9ed90d2eb35,
		0x0680447a8e5ff9a6,
	})

	c.Square(&z)
	wasSquare := c.Equal(a)
	f.CMove(f, &z, wasSquare)
	return f, wasSquare
}

// Invert performs modular inverse
func (f *fp) Invert(a *fp) (*fp, int) {
	// Exponentiate by p - 2
	t := &fp{}
	t.pow(a, &fp{
		0xb9feffffffffaaa9,
		0x1eabfffeb153ffff,
		0x6730d2a0f6b0f624,
		0x64774b84f38512bf,
		0x4b1ba7b6434bacd7,
		0x1a0111ea397fe69a,
	})
	wasInverted := a.IsNonZero()
	f.CMove(a, t, wasInverted)
	return f, wasInverted
}

// SetBytes converts a little endian byte array into a field element
// return 0 if the bytes are not in the field, 1 if they are
func (f *fp) SetBytes(arg *[FieldBytes]byte) (*fp, int) {
	var borrow uint64
	t := &fp{}

	t[0] = binary.LittleEndian.Uint64(arg[:8])
	t[1] = binary.LittleEndian.Uint64(arg[8:16])
	t[2] = binary.LittleEndian.Uint64(arg[16:24])
	t[3] = binary.LittleEndian.Uint64(arg[24:32])
	t[4] = binary.LittleEndian.Uint64(arg[32:40])
	t[5] = binary.LittleEndian.Uint64(arg[40:])

	// Try to subtract the modulus
	_, borrow = sbb(t[0], modulus[0], 0)
	_, borrow = sbb(t[1], modulus[1], borrow)
	_, borrow = sbb(t[2], modulus[2], borrow)
	_, borrow = sbb(t[3], modulus[3], borrow)
	_, borrow = sbb(t[4], modulus[4], borrow)
	_, borrow = sbb(t[5], modulus[5], borrow)

	// If the element is smaller than modulus then the
	// subtraction will underflow, producing a borrow value
	// of 1. Otherwise, it'll be zero.
	mask := int(borrow)
	return f.CMove(f, t.toMontgomery(t), mask), mask
}

// SetBytesWide takes 96 bytes as input and treats them as a 512-bit number.
// Attributed to https://github.com/zcash/pasta_curves/blob/main/src/fields/Fp.rs#L255
// We reduce an arbitrary 768-bit number by decomposing it into two 384-bit digits
// with the higher bits multiplied by 2^384. Thus, we perform two reductions
//
// 1. the lower bits are multiplied by r^2, as normal
// 2. the upper bits are multiplied by r^2 * 2^384 = r^3
//
// and computing their sum in the field. It remains to see that arbitrary 384-bit
// numbers can be placed into Montgomery form safely using the reduction. The
// reduction works so long as the product is less than r=2^384 multiplied by
// the modulus. This holds because for any `c` smaller than the modulus, we have
// that (2^384 - 1)*c is an acceptable product for the reduction. Therefore, the
// reduction always works so long as `c` is in the field; in this case it is either the
// constant `r2` or `r3`.
func (f *fp) SetBytesWide(a *[WideFieldBytes]byte) *fp {
	d0 := &fp{
		binary.LittleEndian.Uint64(a[:8]),
		binary.LittleEndian.Uint64(a[8:16]),
		binary.LittleEndian.Uint64(a[16:24]),
		binary.LittleEndian.Uint64(a[24:32]),
		binary.LittleEndian.Uint64(a[32:40]),
		binary.LittleEndian.Uint64(a[40:48]),
	}
	d1 := &fp{
		binary.LittleEndian.Uint64(a[48:56]),
		binary.LittleEndian.Uint64(a[56:64]),
		binary.LittleEndian.Uint64(a[64:72]),
		binary.LittleEndian.Uint64(a[72:80]),
		binary.LittleEndian.Uint64(a[80:88]),
		binary.LittleEndian.Uint64(a[88:96]),
	}
	// d0*r2 + d1*r3
	d0.Mul(d0, &r2)
	d1.Mul(d1, &r3)
	return f.Add(d0, d1)
}

// SetBigInt initializes an element from big.Int
// The value is reduced by the modulus
func (f *fp) SetBigInt(bi *big.Int) *fp {
	var buffer [FieldBytes]byte
	t := new(big.Int).Set(bi)
	t.Mod(t, biModulus)
	t.FillBytes(buffer[:])
	copy(buffer[:], internal.ReverseScalarBytes(buffer[:]))
	_, _ = f.SetBytes(&buffer)
	return f
}

// Set copies a into fp
func (f *fp) Set(a *fp) *fp {
	f[0] = a[0]
	f[1] = a[1]
	f[2] = a[2]
	f[3] = a[3]
	f[4] = a[4]
	f[5] = a[5]
	return f
}

// SetLimbs converts an array into a field element
// by converting to montgomery form
func (f *fp) SetLimbs(a *[Limbs]uint64) *fp {
	return f.toMontgomery((*fp)(a))
}

// SetRaw converts a raw array into a field element
// Assumes input is already in montgomery form
func (f *fp) SetRaw(a *[Limbs]uint64) *fp {
	f[0] = a[0]
	f[1] = a[1]
	f[2] = a[2]
	f[3] = a[3]
	f[4] = a[4]
	f[5] = a[5]
	return f
}

// Bytes converts a field element to a little endian byte array
func (f *fp) Bytes() [FieldBytes]byte {
	var out [FieldBytes]byte
	t := new(fp).fromMontgomery(f)
	binary.LittleEndian.PutUint64(out[:8], t[0])
	binary.LittleEndian.PutUint64(out[8:16], t[1])
	binary.LittleEndian.PutUint64(out[16:24], t[2])
	binary.LittleEndian.PutUint64(out[24:32], t[3])
	binary.LittleEndian.PutUint64(out[32:40], t[4])
	binary.LittleEndian.PutUint64(out[40:], t[5])
	return out
}

// BigInt converts this element into the big.Int struct
func (f *fp) BigInt() *big.Int {
	buffer := f.Bytes()
	return new(big.Int).SetBytes(internal.ReverseScalarBytes(buffer[:]))
}

// Raw converts this element into the a [FieldLimbs]uint64
func (f *fp) Raw() [Limbs]uint64 {
	t := new(fp).fromMontgomery(f)
	return *t
}

// CMove performs conditional select.
// selects arg1 if choice == 0 and arg2 if choice == 1
func (f *fp) CMove(arg1, arg2 *fp, choice int) *fp {
	mask := uint64(-choice)
	f[0] = arg1[0] ^ ((arg1[0] ^ arg2[0]) & mask)
	f[1] = arg1[1] ^ ((arg1[1] ^ arg2[1]) & mask)
	f[2] = arg1[2] ^ ((arg1[2] ^ arg2[2]) & mask)
	f[3] = arg1[3] ^ ((arg1[3] ^ arg2[3]) & mask)
	f[4] = arg1[4] ^ ((arg1[4] ^ arg2[4]) & mask)
	f[5] = arg1[5] ^ ((arg1[5] ^ arg2[5]) & mask)
	return f
}

// CNeg conditionally negates a if choice == 1
func (f *fp) CNeg(a *fp, choice int) *fp {
	var t fp
	t.Neg(a)
	return f.CMove(f, &t, choice)
}

// Exp raises base^exp.
func (f *fp) Exp(base, exp *fp) *fp {
	e := (&fp{}).fromMontgomery(exp)
	return f.pow(base, e)
}

func (f *fp) pow(base, e *fp) *fp {
	var tmp, res fp
	res.SetOne()

	for i := len(e) - 1; i >= 0; i-- {
		for j := 63; j >= 0; j-- {
			res.Square(&res)
			tmp.Mul(&res, base)
			res.CMove(&res, &tmp, int(e[i]>>j)&1)
		}
	}
	f[0] = res[0]
	f[1] = res[1]
	f[2] = res[2]
	f[3] = res[3]
	f[4] = res[4]
	f[5] = res[5]
	return f
}

// montReduce performs the montgomery reduction
func (f *fp) montReduce(r *[2 * Limbs]uint64) *fp {
	// Taken from Algorithm 14.32 in Handbook of Applied Cryptography
	var r1, r2, r3, r4, r5, r6, r7, r8, r9, r10, r11, carry, k uint64
	var rr fp

	k = r[0] * inv
	_, carry = mac(r[0], k, modulus[0], 0)
	r1, carry = mac(r[1], k, modulus[1], carry)
	r2, carry = mac(r[2], k, modulus[2], carry)
	r3, carry = mac(r[3], k, modulus[3], carry)
	r4, carry = mac(r[4], k, modulus[4], carry)
	r5, carry = mac(r[5], k, modulus[5], carry)
	r6, r7 = adc(r[6], 0, carry)

	k = r1 * inv
	_, carry = mac(r1, k, modulus[0], 0)
	r2, carry = mac(r2, k, modulus[1], carry)
	r3, carry = mac(r3, k, modulus[2], carry)
	r4, carry = mac(r4, k, modulus[3], carry)
	r5, carry = mac(r5, k, modulus[4], carry)
	r6, carry = mac(r6, k, modulus[5], carry)
	r7, r8 = adc(r7, r[7], carry)

	k = r2 * inv
	_, carry = mac(r2, k, modulus[0], 0)
	r3, carry = mac(r3, k, modulus[1], carry)
	r4, carry = mac(r4, k, modulus[2], carry)
	r5, carry = mac(r5, k, modulus[3], carry)
	r6, carry = mac(r6, k, modulus[4], carry)
	r7, carry = mac(r7, k, modulus[5], carry)
	r8, r9 = adc(r8, r[8], carry)

	k = r3 * inv
	_, carry = mac(r3, k, modulus[0], 0)
	r4, carry = mac(r4, k, modulus[1], carry)
	r5, carry = mac(r5, k, modulus[2], carry)
	r6, carry = mac(r6, k, modulus[3], carry)
	r7, carry = mac(r7, k, modulus[4], carry)
	r8, carry = mac(r8, k, modulus[5], carry)
	r9, r10 = adc(r9, r[9], carry)

	k = r4 * inv
	_, carry = mac(r4, k, modulus[0], 0)
	r5, carry = mac(r5, k, modulus[1], carry)
	r6, carry = mac(r6, k, modulus[2], carry)
	r7, carry = mac(r7, k, modulus[3], carry)
	r8, carry = mac(r8, k, modulus[4], carry)
	r9, carry = mac(r9, k, modulus[5], carry)
	r10, r11 = adc(r10, r[10], carry)

	k = r5 * inv
	_, carry = mac(r5, k, modulus[0], 0)
	rr[0], carry = mac(r6, k, modulus[1], carry)
	rr[1], carry = mac(r7, k, modulus[2], carry)
	rr[2], carry = mac(r8, k, modulus[3], carry)
	rr[3], carry = mac(r9, k, modulus[4], carry)
	rr[4], carry = mac(r10, k, modulus[5], carry)
	rr[5], _ = adc(r11, r[11], carry)

	return f.Sub(&rr, &modulus)
}
