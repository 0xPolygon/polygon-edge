package bls12381

import "io"

// fp12 represents an element a + b w of fp^12 = fp^6 / w^2 - v.
type fp12 struct {
	A, B fp6
}

// SetFp creates an element from a lower field
func (f *fp12) SetFp(a *fp) *fp12 {
	f.A.SetFp(a)
	f.B.SetZero()
	return f
}

// SetFp2 creates an element from a lower field
func (f *fp12) SetFp2(a *fp2) *fp12 {
	f.A.SetFp2(a)
	f.B.SetZero()
	return f
}

// SetFp6 creates an element from a lower field
func (f *fp12) SetFp6(a *fp6) *fp12 {
	f.A.Set(a)
	f.B.SetZero()
	return f
}

// Set copies the value `a`
func (f *fp12) Set(a *fp12) *fp12 {
	f.A.Set(&a.A)
	f.B.Set(&a.B)
	return f
}

// SetZero fp6 to zero
func (f *fp12) SetZero() *fp12 {
	f.A.SetZero()
	f.B.SetZero()
	return f
}

// SetOne fp6 to multiplicative identity element
func (f *fp12) SetOne() *fp12 {
	f.A.SetOne()
	f.B.SetZero()
	return f
}

// Random generates a random field element
func (f *fp12) Random(reader io.Reader) (*fp12, error) {
	a, err := new(fp6).Random(reader)
	if err != nil {
		return nil, err
	}
	b, err := new(fp6).Random(reader)
	if err != nil {
		return nil, err
	}
	f.A.Set(a)
	f.B.Set(b)
	return f, nil
}

// Square computes arg^2
func (f *fp12) Square(arg *fp12) *fp12 {
	var ab, apb, aTick, bTick, t fp6

	ab.Mul(&arg.A, &arg.B)
	apb.Add(&arg.A, &arg.B)

	aTick.MulByNonResidue(&arg.B)
	aTick.Add(&aTick, &arg.A)
	aTick.Mul(&aTick, &apb)
	aTick.Sub(&aTick, &ab)
	t.MulByNonResidue(&ab)
	aTick.Sub(&aTick, &t)

	bTick.Double(&ab)

	f.A.Set(&aTick)
	f.B.Set(&bTick)
	return f
}

// Invert computes this element's field inversion
func (f *fp12) Invert(arg *fp12) (*fp12, int) {
	var a, b, t fp6
	a.Square(&arg.A)
	b.Square(&arg.B)
	b.MulByNonResidue(&b)
	a.Sub(&a, &b)
	_, wasInverted := t.Invert(&a)

	a.Mul(&arg.A, &t)
	t.Neg(&t)
	b.Mul(&arg.B, &t)
	f.A.CMove(&f.A, &a, wasInverted)
	f.B.CMove(&f.B, &b, wasInverted)
	return f, wasInverted
}

// Add computes arg1+arg2
func (f *fp12) Add(arg1, arg2 *fp12) *fp12 {
	f.A.Add(&arg1.A, &arg2.A)
	f.B.Add(&arg1.B, &arg2.B)
	return f
}

// Sub computes arg1-arg2
func (f *fp12) Sub(arg1, arg2 *fp12) *fp12 {
	f.A.Sub(&arg1.A, &arg2.A)
	f.B.Sub(&arg1.B, &arg2.B)
	return f
}

// Mul computes arg1*arg2
func (f *fp12) Mul(arg1, arg2 *fp12) *fp12 {
	var aa, bb, a2b2, a, b fp6

	aa.Mul(&arg1.A, &arg2.A)
	bb.Mul(&arg1.B, &arg2.B)
	a2b2.Add(&arg2.A, &arg2.B)
	b.Add(&arg1.A, &arg1.B)
	b.Mul(&b, &a2b2)
	b.Sub(&b, &aa)
	b.Sub(&b, &bb)
	a.MulByNonResidue(&bb)
	a.Add(&a, &aa)

	f.A.Set(&a)
	f.B.Set(&b)
	return f
}

// Neg computes the field negation
func (f *fp12) Neg(arg *fp12) *fp12 {
	f.A.Neg(&arg.A)
	f.B.Neg(&arg.B)
	return f
}

// MulByABD computes arg * a * b * c
func (f *fp12) MulByABD(arg *fp12, a, b, d *fp2) *fp12 {
	var aa, bb, aTick, bTick fp6
	var bd fp2

	aa.MulByAB(&arg.A, a, b)
	bb.MulByB(&arg.B, d)
	bd.Add(b, d)

	bTick.Add(&arg.A, &arg.B)
	bTick.MulByAB(&bTick, a, &bd)
	bTick.Sub(&bTick, &aa)
	bTick.Sub(&bTick, &bb)

	aTick.MulByNonResidue(&bb)
	aTick.Add(&aTick, &aa)

	f.A.Set(&aTick)
	f.B.Set(&bTick)

	return f
}

// Conjugate computes the field conjugation
func (f *fp12) Conjugate(arg *fp12) *fp12 {
	f.A.Set(&arg.A)
	f.B.Neg(&arg.B)
	return f
}

// FrobeniusMap raises this element to p.
func (f *fp12) FrobeniusMap(arg *fp12) *fp12 {
	var a, b, up1epm1div6 fp6

	// (u + 1)^((p - 1) / 6)
	up1epm1div6.A = fp2{
		A: fp{
			0x07089552b319d465,
			0xc6695f92b50a8313,
			0x97e83cccd117228f,
			0xa35baecab2dc29ee,
			0x1ce393ea5daace4d,
			0x08f2220fb0fb66eb,
		},
		B: fp{
			0xb2f66aad4ce5d646,
			0x5842a06bfc497cec,
			0xcf4895d42599d394,
			0xc11b9cba40a8e8d0,
			0x2e3813cbe5a0de89,
			0x110eefda88847faf,
		},
	}

	a.FrobeniusMap(&arg.A)
	b.FrobeniusMap(&arg.B)

	// b' = b' * (u + 1)^((p - 1) / 6)
	b.Mul(&b, &up1epm1div6)

	f.A.Set(&a)
	f.B.Set(&b)
	return f
}

// Equal returns 1 if fp12 == rhs, 0 otherwise
func (f *fp12) Equal(rhs *fp12) int {
	return f.A.Equal(&rhs.A) & f.B.Equal(&rhs.B)
}

// IsZero returns 1 if fp6 == 0, 0 otherwise
func (f *fp12) IsZero() int {
	return f.A.IsZero() & f.B.IsZero()
}

// IsOne returns 1 if fp12 == 1, 0 otherwise
func (f *fp12) IsOne() int {
	return f.A.IsOne() & f.B.IsZero()
}

// CMove performs conditional select.
// selects arg1 if choice == 0 and arg2 if choice == 1
func (f *fp12) CMove(arg1, arg2 *fp12, choice int) *fp12 {
	f.A.CMove(&arg1.A, &arg2.A, choice)
	f.B.CMove(&arg1.B, &arg2.B, choice)
	return f
}
