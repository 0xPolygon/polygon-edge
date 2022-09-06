package bls12381

import (
	"io"
)

// fp2 is a point in p^2
type fp2 struct {
	A, B fp
}

// Set copies a into fp2
func (f *fp2) Set(a *fp2) *fp2 {
	f.A.Set(&a.A)
	f.B.Set(&a.B)
	return f
}

// SetZero fp2 = 0
func (f *fp2) SetZero() *fp2 {
	f.A.SetZero()
	f.B.SetZero()
	return f
}

// SetOne fp2 to the multiplicative identity element
func (f *fp2) SetOne() *fp2 {
	f.A.SetOne()
	f.B.SetZero()
	return f
}

// SetFp creates an element from a lower field
func (f *fp2) SetFp(a *fp) *fp2 {
	f.A.Set(a)
	f.B.SetZero()
	return f
}

// Random generates a random field element
func (f *fp2) Random(reader io.Reader) (*fp2, error) {
	a, err := new(fp).Random(reader)
	if err != nil {
		return nil, err
	}
	b, err := new(fp).Random(reader)
	if err != nil {
		return nil, err
	}
	f.A = *a
	f.B = *b
	return f, nil
}

// IsZero returns 1 if fp2 == 0, 0 otherwise
func (f *fp2) IsZero() int {
	return f.A.IsZero() & f.B.IsZero()
}

// IsOne returns 1 if fp2 == 1, 0 otherwise
func (f *fp2) IsOne() int {
	return f.A.IsOne() & f.B.IsZero()
}

// Equal returns 1 if f == rhs, 0 otherwise
func (f *fp2) Equal(rhs *fp2) int {
	return f.A.Equal(&rhs.A) & f.B.Equal(&rhs.B)
}

// LexicographicallyLargest returns 1 if
// this element is strictly lexicographically larger than its negation
// 0 otherwise
func (f *fp2) LexicographicallyLargest() int {
	// If this element's B coefficient is lexicographically largest
	// then it is lexicographically largest. Otherwise, in the event
	// the B coefficient is zero and the A coefficient is
	// lexicographically largest, then this element is lexicographically
	// largest.

	return f.B.LexicographicallyLargest() |
		f.B.IsZero()&f.A.LexicographicallyLargest()
}

// Sgn0 returns the lowest bit value
func (f *fp2) Sgn0() int {
	// if A = 0 return B.Sgn0  else A.Sgn0
	a := f.A.IsZero()
	t := f.B.Sgn0() & a
	a = -a + 1
	t |= f.A.Sgn0() & a
	return t
}

// FrobeniusMap raises this element to p.
func (f *fp2) FrobeniusMap(a *fp2) *fp2 {
	// This is always just a conjugation. If you're curious why, here's
	// an article about it: https://alicebob.cryptoland.net/the-frobenius-endomorphism-with-finite-fields/
	return f.Conjugate(a)
}

// Conjugate computes the conjugation of this element
func (f *fp2) Conjugate(a *fp2) *fp2 {
	f.A.Set(&a.A)
	f.B.Neg(&a.B)
	return f
}

// MulByNonResidue computes the following:
// multiply a + bu by u + 1, getting
// au + a + bu^2 + bu
// and because u^2 = -1, we get
// (a - b) + (a + b)u
func (f *fp2) MulByNonResidue(a *fp2) *fp2 {
	var aa, bb fp
	aa.Sub(&a.A, &a.B)
	bb.Add(&a.A, &a.B)
	f.A.Set(&aa)
	f.B.Set(&bb)
	return f
}

// Square computes the square of this element
func (f *fp2) Square(arg *fp2) *fp2 {
	var a, b, c fp

	// Complex squaring:
	//
	// v0  = a * b
	// a' = (a + b) * (a + \beta*b) - v0 - \beta * v0
	// b' = 2 * v0
	//
	// In BLS12-381's F_{p^2}, our \beta is -1, so we
	// can modify this formula:
	//
	// a' = (a + b) * (a - b)
	// b' = 2 * a * b
	a.Add(&arg.A, &arg.B)
	b.Sub(&arg.A, &arg.B)
	c.Add(&arg.A, &arg.A)

	f.A.Mul(&a, &b)
	f.B.Mul(&c, &arg.B)
	return f
}

// Add performs field addition
func (f *fp2) Add(arg1, arg2 *fp2) *fp2 {
	f.A.Add(&arg1.A, &arg2.A)
	f.B.Add(&arg1.B, &arg2.B)
	return f
}

// Double doubles specified element
func (f *fp2) Double(a *fp2) *fp2 {
	f.A.Double(&a.A)
	f.B.Double(&a.B)
	return f
}

// Sub performs field subtraction
func (f *fp2) Sub(arg1, arg2 *fp2) *fp2 {
	f.A.Sub(&arg1.A, &arg2.A)
	f.B.Sub(&arg1.B, &arg2.B)
	return f
}

// Mul computes Karatsuba multiplication
func (f *fp2) Mul(arg1, arg2 *fp2) *fp2 {
	var v0, v1, t, a, b fp

	// Karatsuba multiplication:
	//
	// v0  = a0 * b0
	// v1  = a1 * b1
	// c0 = v0 + \beta * v1
	// c1 = (a0 + a1) * (b0 + b1) - v0 - v1
	//
	// In BLS12-381's F_{p^2}, our \beta is -1, so we
	// can modify this formula. (Also, since we always
	// subtract v1, we can compute v1 = -a1 * b1.)
	//
	// v0  = a0 * a1
	// v1  = (-b0) * b1
	// a' = v0 + v1
	// b' = (a0 + b0) * (a1 + b1) - v0 + v1
	v0.Mul(&arg1.A, &arg2.A)
	v1.Mul(new(fp).Neg(&arg1.B), &arg2.B)

	a.Add(&v0, &v1)
	b.Add(&arg1.A, &arg1.B)
	t.Add(&arg2.A, &arg2.B)
	b.Mul(&b, &t)
	b.Sub(&b, &v0)
	b.Add(&b, &v1)
	f.A.Set(&a)
	f.B.Set(&b)
	return f
}

func (f *fp2) Mul0(arg1 *fp2, arg2 *fp) *fp2 {
	f.A.Mul(&arg1.A, arg2)
	f.B.Mul(&arg1.B, arg2)
	return f
}

// MulBy3b returns arg * 12 or 3 * b
func (f *fp2) MulBy3b(arg *fp2) *fp2 {
	return f.Mul(arg, &curveG23B)
}

// Neg performs field negation
func (f *fp2) Neg(a *fp2) *fp2 {
	f.A.Neg(&a.A)
	f.B.Neg(&a.B)
	return f
}

// Sqrt performs field square root
func (f *fp2) Sqrt(a *fp2) (*fp2, int) {
	// Algorithm 9, https://eprint.iacr.org/2012/685.pdf
	// with constant time modifications.
	var a1, alpha, x0, t, res, res2 fp2
	e1 := a.IsZero()
	// a1 = self^((p - 3) / 4)
	a1.pow(a, &[Limbs]uint64{
		0xee7fbfffffffeaaa,
		0x07aaffffac54ffff,
		0xd9cc34a83dac3d89,
		0xd91dd2e13ce144af,
		0x92c6e9ed90d2eb35,
		0x0680447a8e5ff9a6,
	})

	// alpha = a1^2 * a = a^((p - 3) / 2 + 1) = a^((p - 1) / 2)
	alpha.Square(&a1)
	alpha.Mul(&alpha, a)

	// x0 = self^((p + 1) / 4)
	x0.Mul(&a1, a)

	// In the event that alpha = -1, the element is order p - 1. So
	// we're just trying to get the square of an element of the subfield
	// fp. This is given by x0 * u, since u = sqrt(-1). Since the element
	// x0 = a + bu has b = 0, the solution is therefore au.
	res2.A.Neg(&x0.B)
	res2.B.Set(&x0.A)
	// alpha == -1
	e2 := alpha.Equal(&fp2{
		A: fp{
			0x43f5fffffffcaaae,
			0x32b7fff2ed47fffd,
			0x07e83a49a2e99d69,
			0xeca8f3318332bb7a,
			0xef148d1ea0f4c069,
			0x040ab3263eff0206,
		},
		B: fp{},
	})

	// Otherwise, the correct solution is (1 + alpha)^((p - 1) // 2) * x0
	t.SetOne()
	t.Add(&t, &alpha)
	t.pow(&t, &[Limbs]uint64{
		0xdcff7fffffffd555,
		0x0f55ffff58a9ffff,
		0xb39869507b587b12,
		0xb23ba5c279c2895f,
		0x258dd3db21a5d66b,
		0x0d0088f51cbff34d,
	})
	t.Mul(&t, &x0)
	// if a = 0, then its zero
	res.CMove(&res2, &res, e1)
	// if alpha = -1, its not (1 + alpha)^((p - 1) // 2) * x0
	// but au
	res.CMove(&t, &res, e2)

	// is the result^2 = a
	t.Square(&res)
	e3 := t.Equal(a)
	f.CMove(f, &res, e3)
	return f, e3
}

// Invert computes the multiplicative inverse of this field
// element, returning the original value of fp2
// in the case that this element is zero.
func (f *fp2) Invert(arg *fp2) (*fp2, int) {
	// We wish to find the multiplicative inverse of a nonzero
	// element a + bu in fp2. We leverage an identity
	//
	// (a + bu)(a - bu) = a^2 + b^2
	//
	// which holds because u^2 = -1. This can be rewritten as
	//
	// (a + bu)(a - bu)/(a^2 + b^2) = 1
	//
	// because a^2 + b^2 = 0 has no nonzero solutions for (a, b).
	// This gives that (a - bu)/(a^2 + b^2) is the inverse
	// of (a + bu). Importantly, this can be computing using
	// only a single inversion in fp.
	var a, b, t fp
	a.Square(&arg.A)
	b.Square(&arg.B)
	a.Add(&a, &b)
	_, wasInverted := t.Invert(&a)
	// a * t
	a.Mul(&arg.A, &t)
	// b * -t
	b.Neg(&t)
	b.Mul(&b, &arg.B)
	f.A.CMove(&f.A, &a, wasInverted)
	f.B.CMove(&f.B, &b, wasInverted)
	return f, wasInverted
}

// CMove performs conditional select.
// selects arg1 if choice == 0 and arg2 if choice == 1
func (f *fp2) CMove(arg1, arg2 *fp2, choice int) *fp2 {
	f.A.CMove(&arg1.A, &arg2.A, choice)
	f.B.CMove(&arg1.B, &arg2.B, choice)
	return f
}

// CNeg conditionally negates a if choice == 1
func (f *fp2) CNeg(a *fp2, choice int) *fp2 {
	var t fp2
	t.Neg(a)
	return f.CMove(f, &t, choice)
}

func (f *fp2) pow(base *fp2, exp *[Limbs]uint64) *fp2 {
	res := (&fp2{}).SetOne()
	tmp := (&fp2{}).SetZero()

	for i := len(exp) - 1; i >= 0; i-- {
		for j := 63; j >= 0; j-- {
			res.Square(res)
			tmp.Mul(res, base)
			res.CMove(res, tmp, int(exp[i]>>j)&1)
		}
	}
	return f.Set(res)
}
