package native

import (
	"encoding/binary"
	"fmt"
	"math/big"

	"github.com/coinbase/kryptology/internal"
)

// FieldLimbs is the number of limbs needed to represent this field
const FieldLimbs = 4

// FieldBytes is the number of bytes needed to represent this field
const FieldBytes = 32

// WideFieldBytes is the number of bytes needed for safe conversion
// to this field to avoid bias when reduced
const WideFieldBytes = 64

// Field represents a field element
type Field struct {
	// Value is the field elements value
	Value [FieldLimbs]uint64
	// Params are the field parameters
	Params *FieldParams
	// Arithmetic are the field methods
	Arithmetic FieldArithmetic
}

// FieldParams are the field parameters
type FieldParams struct {
	// R is 2^256 mod Modulus
	R [FieldLimbs]uint64
	// R2 is 2^512 mod Modulus
	R2 [FieldLimbs]uint64
	// R3 is 2^768 mod Modulus
	R3 [FieldLimbs]uint64
	// Modulus of the field
	Modulus [FieldLimbs]uint64
	// Modulus as big.Int
	BiModulus *big.Int
}

// FieldArithmetic are the methods that can be done on a field
type FieldArithmetic interface {
	// ToMontgomery converts this field to montgomery form
	ToMontgomery(out, arg *[FieldLimbs]uint64)
	// FromMontgomery converts this field from montgomery form
	FromMontgomery(out, arg *[FieldLimbs]uint64)
	// Neg performs modular negation
	Neg(out, arg *[FieldLimbs]uint64)
	// Square performs modular square
	Square(out, arg *[FieldLimbs]uint64)
	// Mul performs modular multiplication
	Mul(out, arg1, arg2 *[FieldLimbs]uint64)
	// Add performs modular addition
	Add(out, arg1, arg2 *[FieldLimbs]uint64)
	// Sub performs modular subtraction
	Sub(out, arg1, arg2 *[FieldLimbs]uint64)
	// Sqrt performs modular square root
	Sqrt(wasSquare *int, out, arg *[FieldLimbs]uint64)
	// Invert performs modular inverse
	Invert(wasInverted *int, out, arg *[FieldLimbs]uint64)
	// FromBytes converts a little endian byte array into a field element
	FromBytes(out *[FieldLimbs]uint64, arg *[FieldBytes]byte)
	// ToBytes converts a field element to a little endian byte array
	ToBytes(out *[FieldBytes]byte, arg *[FieldLimbs]uint64)
	// Selectznz performs conditional select.
	// selects arg1 if choice == 0 and arg2 if choice == 1
	Selectznz(out, arg1, arg2 *[FieldLimbs]uint64, choice int)
}

// Cmp returns -1 if f < rhs
// 0 if f == rhs
// 1 if f > rhs
func (f *Field) Cmp(rhs *Field) int {
	return cmpHelper(&f.Value, &rhs.Value)
}

// cmpHelper returns -1 if lhs < rhs
// -1 if lhs == rhs
// 1 if lhs > rhs
// Public only for convenience for some internal implementations
func cmpHelper(lhs, rhs *[FieldLimbs]uint64) int {
	gt := uint64(0)
	lt := uint64(0)
	for i := 3; i >= 0; i-- {
		// convert to two 64-bit numbers where
		// the leading bits are zeros and hold no meaning
		//  so rhs - fp actually means gt
		// and fp - rhs actually means lt.
		rhsH := rhs[i] >> 32
		rhsL := rhs[i] & 0xffffffff
		lhsH := lhs[i] >> 32
		lhsL := lhs[i] & 0xffffffff

		// Check the leading bit
		// if negative then fp > rhs
		// if positive then fp < rhs
		gt |= (rhsH - lhsH) >> 32 & 1 &^ lt
		lt |= (lhsH - rhsH) >> 32 & 1 &^ gt
		gt |= (rhsL - lhsL) >> 32 & 1 &^ lt
		lt |= (lhsL - rhsL) >> 32 & 1 &^ gt
	}
	// Make the result -1 for <, 0 for =, 1 for >
	return int(gt) - int(lt)
}

// Equal returns 1 if f == rhs, 0 otherwise
func (f *Field) Equal(rhs *Field) int {
	return equalHelper(&f.Value, &rhs.Value)
}

func equalHelper(lhs, rhs *[FieldLimbs]uint64) int {
	t := lhs[0] ^ rhs[0]
	t |= lhs[1] ^ rhs[1]
	t |= lhs[2] ^ rhs[2]
	t |= lhs[3] ^ rhs[3]
	return int(((int64(t) | int64(-t)) >> 63) + 1)
}

// IsZero returns 1 if f == 0, 0 otherwise
func (f *Field) IsZero() int {
	t := f.Value[0]
	t |= f.Value[1]
	t |= f.Value[2]
	t |= f.Value[3]
	return int(((int64(t) | int64(-t)) >> 63) + 1)
}

// IsNonZero returns 1 if f != 0, 0 otherwise
func (f *Field) IsNonZero() int {
	t := f.Value[0]
	t |= f.Value[1]
	t |= f.Value[2]
	t |= f.Value[3]
	return int(-((int64(t) | int64(-t)) >> 63))
}

// IsOne returns 1 if f == 1, 0 otherwise
func (f *Field) IsOne() int {
	return equalHelper(&f.Value, &f.Params.R)
}

// Set f = rhs
func (f *Field) Set(rhs *Field) *Field {
	f.Value[0] = rhs.Value[0]
	f.Value[1] = rhs.Value[1]
	f.Value[2] = rhs.Value[2]
	f.Value[3] = rhs.Value[3]
	f.Params = rhs.Params
	f.Arithmetic = rhs.Arithmetic
	return f
}

// SetUint64 f = rhs
func (f *Field) SetUint64(rhs uint64) *Field {
	t := &[FieldLimbs]uint64{rhs, 0, 0, 0}
	f.Arithmetic.ToMontgomery(&f.Value, t)
	return f
}

// SetOne f = r
func (f *Field) SetOne() *Field {
	f.Value[0] = f.Params.R[0]
	f.Value[1] = f.Params.R[1]
	f.Value[2] = f.Params.R[2]
	f.Value[3] = f.Params.R[3]
	return f
}

// SetZero f = 0
func (f *Field) SetZero() *Field {
	f.Value[0] = 0
	f.Value[1] = 0
	f.Value[2] = 0
	f.Value[3] = 0
	return f
}

// SetBytesWide takes 64 bytes as input and treats them as a 512-bit number.
// Attributed to https://github.com/zcash/pasta_curves/blob/main/src/fields/Fp.rs#L255
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
func (f *Field) SetBytesWide(input *[WideFieldBytes]byte) *Field {
	d0 := [FieldLimbs]uint64{
		binary.LittleEndian.Uint64(input[:8]),
		binary.LittleEndian.Uint64(input[8:16]),
		binary.LittleEndian.Uint64(input[16:24]),
		binary.LittleEndian.Uint64(input[24:32]),
	}
	d1 := [FieldLimbs]uint64{
		binary.LittleEndian.Uint64(input[32:40]),
		binary.LittleEndian.Uint64(input[40:48]),
		binary.LittleEndian.Uint64(input[48:56]),
		binary.LittleEndian.Uint64(input[56:64]),
	}
	//f.Arithmetic.ToMontgomery(&d0, &d0)
	//f.Arithmetic.Mul(&d1, &d1, &f.Params.R2)
	//f.Arithmetic.Add(&f.Value, &d0, &d0)
	// Convert to Montgomery form
	tv1 := &[FieldLimbs]uint64{}
	tv2 := &[FieldLimbs]uint64{}
	// d0*r2 + d1*r3
	f.Arithmetic.Mul(tv1, &d0, &f.Params.R2)
	f.Arithmetic.Mul(tv2, &d1, &f.Params.R3)
	f.Arithmetic.Add(&f.Value, tv1, tv2)
	return f
}

// SetBytes attempts to convert a little endian byte representation
// of a scalar into a `Fp`, failing if input is not canonical
func (f *Field) SetBytes(input *[FieldBytes]byte) (*Field, error) {
	d0 := [FieldLimbs]uint64{0, 0, 0, 0}
	f.Arithmetic.FromBytes(&d0, input)

	if cmpHelper(&d0, &f.Params.Modulus) != -1 {
		return nil, fmt.Errorf("invalid byte sequence")
	}
	return f.SetLimbs(&d0), nil
}

// SetBigInt initializes an element from big.Int
// The value is reduced by the modulus
func (f *Field) SetBigInt(bi *big.Int) *Field {
	var buffer [FieldBytes]byte
	t := new(big.Int).Set(bi)
	t.Mod(t, f.Params.BiModulus)
	t.FillBytes(buffer[:])
	copy(buffer[:], internal.ReverseScalarBytes(buffer[:]))
	_, _ = f.SetBytes(&buffer)
	return f
}

// SetRaw converts a raw array into a field element
// Assumes input is already in montgomery form
func (f *Field) SetRaw(input *[FieldLimbs]uint64) *Field {
	f.Value[0] = input[0]
	f.Value[1] = input[1]
	f.Value[2] = input[2]
	f.Value[3] = input[3]
	return f
}

// SetLimbs converts an array into a field element
// by converting to montgomery form
func (f *Field) SetLimbs(input *[FieldLimbs]uint64) *Field {
	f.Arithmetic.ToMontgomery(&f.Value, input)
	return f
}

// Bytes converts this element into a byte representation
// in little endian byte order
func (f *Field) Bytes() [FieldBytes]byte {
	var output [FieldBytes]byte
	tv := &[FieldLimbs]uint64{}
	f.Arithmetic.FromMontgomery(tv, &f.Value)
	f.Arithmetic.ToBytes(&output, tv)
	return output
}

// BigInt converts this element into the big.Int struct
func (f *Field) BigInt() *big.Int {
	buffer := f.Bytes()
	return new(big.Int).SetBytes(internal.ReverseScalarBytes(buffer[:]))
}

// Raw converts this element into the a [FieldLimbs]uint64
func (f *Field) Raw() [FieldLimbs]uint64 {
	res := &[FieldLimbs]uint64{}
	f.Arithmetic.FromMontgomery(res, &f.Value)
	return *res
}

// Double this element
func (f *Field) Double(a *Field) *Field {
	f.Arithmetic.Add(&f.Value, &a.Value, &a.Value)
	return f
}

// Square this element
func (f *Field) Square(a *Field) *Field {
	f.Arithmetic.Square(&f.Value, &a.Value)
	return f
}

// Sqrt this element, if it exists. If true, then value
// is a square root. If false, value is a QNR
func (f *Field) Sqrt(a *Field) (*Field, bool) {
	wasSquare := 0
	f.Arithmetic.Sqrt(&wasSquare, &f.Value, &a.Value)
	return f, wasSquare == 1
}

// Invert this element i.e. compute the multiplicative inverse
// return false, zero if this element is zero.
func (f *Field) Invert(a *Field) (*Field, bool) {
	wasInverted := 0
	f.Arithmetic.Invert(&wasInverted, &f.Value, &a.Value)
	return f, wasInverted == 1
}

// Mul returns the result from multiplying this element by rhs
func (f *Field) Mul(lhs, rhs *Field) *Field {
	f.Arithmetic.Mul(&f.Value, &lhs.Value, &rhs.Value)
	return f
}

// Sub returns the result from subtracting rhs from this element
func (f *Field) Sub(lhs, rhs *Field) *Field {
	f.Arithmetic.Sub(&f.Value, &lhs.Value, &rhs.Value)
	return f
}

// Add returns the result from adding rhs to this element
func (f *Field) Add(lhs, rhs *Field) *Field {
	f.Arithmetic.Add(&f.Value, &lhs.Value, &rhs.Value)
	return f
}

// Neg returns negation of this element
func (f *Field) Neg(input *Field) *Field {
	f.Arithmetic.Neg(&f.Value, &input.Value)
	return f
}

// Exp raises base^exp
func (f *Field) Exp(base, exp *Field) *Field {
	e := [FieldLimbs]uint64{}
	f.Arithmetic.FromMontgomery(&e, &exp.Value)
	Pow(&f.Value, &base.Value, &e, f.Params, f.Arithmetic)
	return f
}

// CMove sets f = lhs if choice == 0 and f = rhs if choice == 1
func (f *Field) CMove(lhs, rhs *Field, choice int) *Field {
	f.Arithmetic.Selectznz(&f.Value, &lhs.Value, &rhs.Value, choice)
	return f
}

// Pow raises base^exp. The result is written to out.
// Public only for convenience for some internal implementations
func Pow(out, base, exp *[FieldLimbs]uint64, params *FieldParams, arithmetic FieldArithmetic) {
	res := [FieldLimbs]uint64{params.R[0], params.R[1], params.R[2], params.R[3]}
	tmp := [FieldLimbs]uint64{}

	for i := len(exp) - 1; i >= 0; i-- {
		for j := 63; j >= 0; j-- {
			arithmetic.Square(&res, &res)
			arithmetic.Mul(&tmp, &res, base)
			arithmetic.Selectznz(&res, &res, &tmp, int(exp[i]>>j)&1)
		}
	}
	out[0] = res[0]
	out[1] = res[1]
	out[2] = res[2]
	out[3] = res[3]
}

// Pow2k raises arg to the power `2^k`. This result is written to out.
// Public only for convenience for some internal implementations
func Pow2k(out, arg *[FieldLimbs]uint64, k int, arithmetic FieldArithmetic) {
	var t [FieldLimbs]uint64
	t[0] = arg[0]
	t[1] = arg[1]
	t[2] = arg[2]
	t[3] = arg[3]
	for i := 0; i < k; i++ {
		arithmetic.Square(&t, &t)
	}

	out[0] = t[0]
	out[1] = t[1]
	out[2] = t[2]
	out[3] = t[3]
}
