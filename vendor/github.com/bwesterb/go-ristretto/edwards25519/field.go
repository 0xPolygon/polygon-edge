package edwards25519

import (
	// Requires for FieldElement.[Set]BigInt().  Obviously not used for actual
	// implementation, as operations on big.Ints are  not constant-time.
	"math/big"

	"encoding/binary"
)

// Set fe to i, the root of -1.  Returns fe.
func (fe *FieldElement) SetI() *FieldElement {
	copy(fe[:], feI[:])
	return fe
}

// Set fe to 0.  Returns fe.
func (fe *FieldElement) SetZero() *FieldElement {
	copy(fe[:], feZero[:])
	return fe
}

// Set fe to 1.  Returns fe.
func (fe *FieldElement) SetOne() *FieldElement {
	copy(fe[:], feOne[:])
	return fe
}

// Sets fe to 2*a without normalizing.  Returns fe.
func (fe *FieldElement) double(a *FieldElement) *FieldElement {
	return fe.add(a, a)
}

// Sets fe to a + b.  Returns fe.
func (fe *FieldElement) Add(a, b *FieldElement) *FieldElement {
	return fe.add(a, b).normalize()
}

// Sets fe to a - b.  Returns fe.
func (fe *FieldElement) Sub(a, b *FieldElement) *FieldElement {
	return fe.sub(a, b).normalize()
}

// Sets fe to a.  Returns fe.
func (fe *FieldElement) Set(a *FieldElement) *FieldElement {
	copy(fe[:], a[:])
	return fe
}

// Returns little endian representation of fe.
func (fe *FieldElement) Bytes() [32]byte {
	var ret [32]byte
	fe.BytesInto(&ret)
	return ret
}

// Set fe to the inverse of a.  Return fe.
func (fe *FieldElement) Inverse(a *FieldElement) *FieldElement {
	var t0, t1, t2, t3 FieldElement
	var i int

	t0.Square(a)
	t1.Square(&t0)
	t1.Square(&t1)
	t1.Mul(a, &t1)
	t0.Mul(&t0, &t1)
	t2.Square(&t0)
	t1.Mul(&t1, &t2)
	t2.Square(&t1)
	for i = 1; i < 5; i++ {
		t2.Square(&t2)
	}
	t1.Mul(&t2, &t1)
	t2.Square(&t1)
	for i = 1; i < 10; i++ {
		t2.Square(&t2)
	}
	t2.Mul(&t2, &t1)
	t3.Square(&t2)
	for i = 1; i < 20; i++ {
		t3.Square(&t3)
	}
	t2.Mul(&t3, &t2)
	t2.Square(&t2)
	for i = 1; i < 10; i++ {
		t2.Square(&t2)
	}
	t1.Mul(&t2, &t1)
	t2.Square(&t1)
	for i = 1; i < 50; i++ {
		t2.Square(&t2)
	}
	t2.Mul(&t2, &t1)
	t3.Square(&t2)
	for i = 1; i < 100; i++ {
		t3.Square(&t3)
	}
	t2.Mul(&t3, &t2)
	t2.Square(&t2)
	for i = 1; i < 50; i++ {
		t2.Square(&t2)
	}
	t1.Mul(&t2, &t1)
	t1.Square(&t1)
	for i = 1; i < 5; i++ {
		t1.Square(&t1)
	}
	return fe.Mul(&t1, &t0)
}

// Set fe to -x if x is negative and x otherwise.  Returns fe.
func (fe *FieldElement) Abs(x *FieldElement) *FieldElement {
	var xNeg FieldElement
	xNeg.Neg(x)
	fe.Set(x)
	fe.ConditionalSet(&xNeg, x.IsNegativeI())
	return fe
}

// Returns 1 if fe is negative, otherwise 0.
func (fe *FieldElement) IsNegativeI() int32 {
	var buf [32]byte
	fe.BytesInto(&buf)
	return int32(buf[0] & 1)
}

// Returns 1 if fe is non-zero, otherwise 0.
func (fe *FieldElement) IsNonZeroI() int32 {
	var buf [32]byte
	fe.BytesInto(&buf)
	ret := (binary.LittleEndian.Uint64(buf[0:8]) |
		binary.LittleEndian.Uint64(buf[8:16]) |
		binary.LittleEndian.Uint64(buf[16:24]) |
		binary.LittleEndian.Uint64(buf[24:32]))
	ret |= ret >> 32
	ret |= ret >> 16
	ret |= ret >> 8
	ret |= ret >> 4
	ret |= ret >> 2
	ret |= ret >> 1
	return int32(ret & 1)
}

// Returns 1 if fe is equal to one, otherwise 0.
func (fe *FieldElement) IsOneI() int32 {
	var b FieldElement
	return 1 - b.sub(fe, &feOne).IsNonZeroI()
}

// Returns 1 if fe is equal to a, otherwise 0.
func (fe *FieldElement) EqualsI(a *FieldElement) int32 {
	var b FieldElement
	return 1 - b.sub(fe, a).IsNonZeroI()
}

// Returns whether fe equals a.
func (fe *FieldElement) Equals(a *FieldElement) bool {
	var b FieldElement
	return b.sub(fe, a).IsNonZeroI() == 0
}

// Returns fe as a big.Int.
//
// WARNING Operations on big.Ints are not constant-time: do not use them
//         for cryptography unless you're sure this is not an issue.
func (fe *FieldElement) BigInt() *big.Int {
	var ret big.Int
	var buf, rBuf [32]byte
	fe.BytesInto(&buf)
	for i := 0; i < 32; i++ {
		rBuf[i] = buf[31-i]
	}
	return ret.SetBytes(rBuf[:])
}

// Writes fe as a string for debugging.
//
// WARNING This operation is not constant-time.  Do not use for cryptography
//         unless you're sure this is not an issue.
func (fe FieldElement) String() string {
	return fe.BigInt().String()
}

// Sets fe to x modulo 2^255-19.
//
// WARNING Operations on big.Ints are not constant-time: do not use them
//         for cryptography unless you're sure this is not an issue.
func (fe *FieldElement) SetBigInt(x *big.Int) *FieldElement {
	var v, bi25519 big.Int
	bi25519.SetString(
		"7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffed", 16)
	buf := v.Mod(x, &bi25519).Bytes()
	var rBuf [32]byte
	for i := 0; i < len(buf) && i < 32; i++ {
		rBuf[i] = buf[len(buf)-i-1]
	}
	return fe.SetBytes(&rBuf)
}

// Sets fe to a^( ((2^255 - 19) - 5) / 8 ) = a^(2^252 - 3).  Returns fe.
//
// This method is useful to compute (inverse) square-roots
// with the method of Lagrange.
func (fe *FieldElement) Exp22523(a *FieldElement) *FieldElement {
	var t0, t1, t2 FieldElement
	var i int

	t0.Square(a)
	for i = 1; i < 1; i++ {
		t0.Square(&t0)
	}
	t1.Square(&t0)
	for i = 1; i < 2; i++ {
		t1.Square(&t1)
	}
	t1.Mul(a, &t1)
	t0.Mul(&t0, &t1)
	t0.Square(&t0)
	for i = 1; i < 1; i++ {
		t0.Square(&t0)
	}
	t0.Mul(&t1, &t0)
	t1.Square(&t0)
	for i = 1; i < 5; i++ {
		t1.Square(&t1)
	}
	t0.Mul(&t1, &t0)
	t1.Square(&t0)
	for i = 1; i < 10; i++ {
		t1.Square(&t1)
	}
	t1.Mul(&t1, &t0)
	t2.Square(&t1)
	for i = 1; i < 20; i++ {
		t2.Square(&t2)
	}
	t1.Mul(&t2, &t1)
	t1.Square(&t1)
	for i = 1; i < 10; i++ {
		t1.Square(&t1)
	}
	t0.Mul(&t1, &t0)
	t1.Square(&t0)
	for i = 1; i < 50; i++ {
		t1.Square(&t1)
	}
	t1.Mul(&t1, &t0)
	t2.Square(&t1)
	for i = 1; i < 100; i++ {
		t2.Square(&t2)
	}
	t1.Mul(&t2, &t1)
	t1.Square(&t1)
	for i = 1; i < 50; i++ {
		t1.Square(&t1)
	}
	t0.Mul(&t1, &t0)
	t0.Square(&t0)
	for i = 1; i < 2; i++ {
		t0.Square(&t0)
	}
	return fe.Mul(&t0, a)
}

// Sets fe to 1/sqrt(a).  Requires a to be a square.  Returns fe.
func (fe *FieldElement) InvSqrt(a *FieldElement) *FieldElement {
	var den2, den3, den4, den6, chk, t, t2 FieldElement
	den2.Square(a)
	den3.Mul(&den2, a)
	den4.Square(&den2)
	den6.Mul(&den2, &den4)
	t.Mul(&den6, a)
	t.Exp22523(&t)
	t.Mul(&t, &den3)
	t2.Mul(&t, &feI)

	chk.Square(&t)
	chk.Mul(&chk, a)

	fe.Set(&t)
	fe.ConditionalSet(&t2, 1-chk.IsOneI())
	return fe
}

// Sets fe to sqrt(a).  Requires a to be a square.  Returns fe.
func (fe *FieldElement) Sqrt(a *FieldElement) *FieldElement {
	var aCopy FieldElement
	aCopy.Set(a)
	fe.InvSqrt(a)
	fe.Mul(fe, &aCopy)

	var feNeg FieldElement
	feNeg.Neg(fe)
	fe.ConditionalSet(&feNeg, fe.IsNegativeI())
	return fe
}

// Sets fe to either 1/sqrt(a) or 1/sqrt(i*a).  Returns 1 in the former case
// and 0 in the latter.
func (fe *FieldElement) InvSqrtI(a *FieldElement) int32 {
	var inCaseA, inCaseB, inCaseD int32
	var den2, den3, den4, den6, chk, t, corr FieldElement
	den2.Square(a)
	den3.Mul(&den2, a)
	den4.Square(&den2)
	den6.Mul(&den2, &den4)
	t.Mul(&den6, a)
	t.Exp22523(&t)
	t.Mul(&t, &den3)

	// case       A           B            C             D
	// ---------------------------------------------------------------
	// t          1/sqrt(a)   -i/sqrt(a)   1/sqrt(i*a)   -i/sqrt(i*a)
	// chk        1           -1           -i            i
	// corr       1           i            1             i
	// ret        1           1            0             0

	chk.Square(&t)
	chk.Mul(&chk, a)

	inCaseA = chk.IsOneI()
	inCaseD = chk.EqualsI(&feI)
	chk.Neg(&chk)
	inCaseB = chk.IsOneI()

	corr.SetOne()
	corr.ConditionalSet(&feI, inCaseB+inCaseD)
	t.Mul(&t, &corr)
	fe.Set(&t)

	return inCaseA + inCaseB
}

// Returns 1 if b == c and 0 otherwise.  Assumes 0 <= b, c < 2^30.
func equal30(b, c int32) int32 {
	x := uint32(b ^ c)
	x--
	return int32(x >> 31)
}

// Returns 1 if b == c and 0 otherwise.  Assumes 2^15  <= b, c < 2^30.
func equal15(b, c int32) int32 {
	ub := uint16(b)
	uc := uint16(c)
	x := uint32(ub ^ uc)
	x--
	return int32(x >> 31)
}

// Returns 1 if b < 0 and 0 otherwise.
func negative(b int32) int32 {
	return (b >> 31) & 1
}
