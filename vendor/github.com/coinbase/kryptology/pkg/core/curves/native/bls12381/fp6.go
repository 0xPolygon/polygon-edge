package bls12381

import "io"

// fp6 represents an element
// a + b v + c v^2 of fp^6 = fp^2 / v^3 - u - 1.
type fp6 struct {
	A, B, C fp2
}

// Set fp6 = a
func (f *fp6) Set(a *fp6) *fp6 {
	f.A.Set(&a.A)
	f.B.Set(&a.B)
	f.C.Set(&a.C)
	return f
}

// SetFp creates an element from a lower field
func (f *fp6) SetFp(a *fp) *fp6 {
	f.A.SetFp(a)
	f.B.SetZero()
	f.C.SetZero()
	return f
}

// SetFp2 creates an element from a lower field
func (f *fp6) SetFp2(a *fp2) *fp6 {
	f.A.Set(a)
	f.B.SetZero()
	f.C.SetZero()
	return f
}

// SetZero fp6 to zero
func (f *fp6) SetZero() *fp6 {
	f.A.SetZero()
	f.B.SetZero()
	f.C.SetZero()
	return f
}

// SetOne fp6 to multiplicative identity element
func (f *fp6) SetOne() *fp6 {
	f.A.SetOne()
	f.B.SetZero()
	f.C.SetZero()
	return f
}

// Random generates a random field element
func (f *fp6) Random(reader io.Reader) (*fp6, error) {
	a, err := new(fp2).Random(reader)
	if err != nil {
		return nil, err
	}
	b, err := new(fp2).Random(reader)
	if err != nil {
		return nil, err
	}
	c, err := new(fp2).Random(reader)
	if err != nil {
		return nil, err
	}
	f.A.Set(a)
	f.B.Set(b)
	f.C.Set(c)
	return f, nil
}

// Add computes arg1+arg2
func (f *fp6) Add(arg1, arg2 *fp6) *fp6 {
	f.A.Add(&arg1.A, &arg2.A)
	f.B.Add(&arg1.B, &arg2.B)
	f.C.Add(&arg1.C, &arg2.C)
	return f
}

// Double computes arg1+arg1
func (f *fp6) Double(arg *fp6) *fp6 {
	return f.Add(arg, arg)
}

// Sub computes arg1-arg2
func (f *fp6) Sub(arg1, arg2 *fp6) *fp6 {
	f.A.Sub(&arg1.A, &arg2.A)
	f.B.Sub(&arg1.B, &arg2.B)
	f.C.Sub(&arg1.C, &arg2.C)
	return f
}

// Mul computes arg1*arg2
func (f *fp6) Mul(arg1, arg2 *fp6) *fp6 {
	var aa, bb, cc, s, t1, t2, t3 fp2

	aa.Mul(&arg1.A, &arg2.A)
	bb.Mul(&arg1.B, &arg2.B)
	cc.Mul(&arg1.C, &arg2.C)

	t1.Add(&arg2.B, &arg2.C)
	s.Add(&arg1.B, &arg1.C)
	t1.Mul(&t1, &s)
	t1.Sub(&t1, &bb)
	t1.Sub(&t1, &cc)
	t1.MulByNonResidue(&t1)
	t1.Add(&t1, &aa)

	t3.Add(&arg2.A, &arg2.C)
	s.Add(&arg1.A, &arg1.C)
	t3.Mul(&t3, &s)
	t3.Sub(&t3, &aa)
	t3.Add(&t3, &bb)
	t3.Sub(&t3, &cc)

	t2.Add(&arg2.A, &arg2.B)
	s.Add(&arg1.A, &arg1.B)
	t2.Mul(&t2, &s)
	t2.Sub(&t2, &aa)
	t2.Sub(&t2, &bb)
	cc.MulByNonResidue(&cc)
	t2.Add(&t2, &cc)

	f.A.Set(&t1)
	f.B.Set(&t2)
	f.C.Set(&t3)
	return f
}

// MulByB scales this field by a scalar in the B coefficient
func (f *fp6) MulByB(arg *fp6, b *fp2) *fp6 {
	var bB, t1, t2 fp2
	bB.Mul(&arg.B, b)
	// (b + c) * arg2 - bB
	t1.Add(&arg.B, &arg.C)
	t1.Mul(&t1, b)
	t1.Sub(&t1, &bB)
	t1.MulByNonResidue(&t1)

	t2.Add(&arg.A, &arg.B)
	t2.Mul(&t2, b)
	t2.Sub(&t2, &bB)

	f.A.Set(&t1)
	f.B.Set(&t2)
	f.C.Set(&bB)
	return f
}

// MulByAB scales this field by scalars in the A and B coefficients
func (f *fp6) MulByAB(arg *fp6, a, b *fp2) *fp6 {
	var aA, bB, t1, t2, t3 fp2

	aA.Mul(&arg.A, a)
	bB.Mul(&arg.B, b)

	t1.Add(&arg.B, &arg.C)
	t1.Mul(&t1, b)
	t1.Sub(&t1, &bB)
	t1.MulByNonResidue(&t1)
	t1.Add(&t1, &aA)

	t2.Add(a, b)
	t3.Add(&arg.A, &arg.B)
	t2.Mul(&t2, &t3)
	t2.Sub(&t2, &aA)
	t2.Sub(&t2, &bB)

	t3.Add(&arg.A, &arg.C)
	t3.Mul(&t3, a)
	t3.Sub(&t3, &aA)
	t3.Add(&t3, &bB)

	f.A.Set(&t1)
	f.B.Set(&t2)
	f.C.Set(&t3)

	return f
}

// MulByNonResidue multiplies by quadratic nonresidue v.
func (f *fp6) MulByNonResidue(arg *fp6) *fp6 {
	// Given a + bv + cv^2, this produces
	//     av + bv^2 + cv^3
	// but because v^3 = u + 1, we have
	//     c(u + 1) + av + bv^2
	var a, b, c fp2
	a.MulByNonResidue(&arg.C)
	b.Set(&arg.A)
	c.Set(&arg.B)
	f.A.Set(&a)
	f.B.Set(&b)
	f.C.Set(&c)
	return f
}

// FrobeniusMap raises this element to p.
func (f *fp6) FrobeniusMap(arg *fp6) *fp6 {
	var a, b, c fp2
	pm1Div3 := fp2{
		A: fp{},
		B: fp{
			0xcd03c9e48671f071,
			0x5dab22461fcda5d2,
			0x587042afd3851b95,
			0x8eb60ebe01bacb9e,
			0x03f97d6e83d050d2,
			0x18f0206554638741,
		},
	}
	p2m2Div3 := fp2{
		A: fp{
			0x890dc9e4867545c3,
			0x2af322533285a5d5,
			0x50880866309b7e2c,
			0xa20d1b8c7e881024,
			0x14e4f04fe2db9068,
			0x14e56d3f1564853a,
		},
		B: fp{},
	}
	a.FrobeniusMap(&arg.A)
	b.FrobeniusMap(&arg.B)
	c.FrobeniusMap(&arg.C)

	// b = b * (u + 1)^((p - 1) / 3)
	b.Mul(&b, &pm1Div3)

	// c = c * (u + 1)^((2p - 2) / 3)
	c.Mul(&c, &p2m2Div3)

	f.A.Set(&a)
	f.B.Set(&b)
	f.C.Set(&c)
	return f
}

// Square computes fp6^2
func (f *fp6) Square(arg *fp6) *fp6 {
	var s0, s1, s2, s3, s4, ab, bc fp2

	s0.Square(&arg.A)
	ab.Mul(&arg.A, &arg.B)
	s1.Double(&ab)
	s2.Sub(&arg.A, &arg.B)
	s2.Add(&s2, &arg.C)
	s2.Square(&s2)
	bc.Mul(&arg.B, &arg.C)
	s3.Double(&bc)
	s4.Square(&arg.C)

	f.A.MulByNonResidue(&s3)
	f.A.Add(&f.A, &s0)

	f.B.MulByNonResidue(&s4)
	f.B.Add(&f.B, &s1)

	// s1 + s2 + s3 - s0 - s4
	f.C.Add(&s1, &s2)
	f.C.Add(&f.C, &s3)
	f.C.Sub(&f.C, &s0)
	f.C.Sub(&f.C, &s4)

	return f
}

// Invert computes this element's field inversion
func (f *fp6) Invert(arg *fp6) (*fp6, int) {
	var a, b, c, s, t fp2

	// a' = a^2 - (b * c).mul_by_nonresidue()
	a.Mul(&arg.B, &arg.C)
	a.MulByNonResidue(&a)
	t.Square(&arg.A)
	a.Sub(&t, &a)

	// b' = (c^2).mul_by_nonresidue() - (a * b)
	b.Square(&arg.C)
	b.MulByNonResidue(&b)
	t.Mul(&arg.A, &arg.B)
	b.Sub(&b, &t)

	// c' = b^2 - (a * c)
	c.Square(&arg.B)
	t.Mul(&arg.A, &arg.C)
	c.Sub(&c, &t)

	// t = ((b * c') + (c * b')).mul_by_nonresidue() + (a * a')
	s.Mul(&arg.B, &c)
	t.Mul(&arg.C, &b)
	s.Add(&s, &t)
	s.MulByNonResidue(&s)

	t.Mul(&arg.A, &a)
	s.Add(&s, &t)

	_, wasInverted := t.Invert(&s)

	// newA = a' * t^-1
	s.Mul(&a, &t)
	f.A.CMove(&f.A, &s, wasInverted)
	// newB = b' * t^-1
	s.Mul(&b, &t)
	f.B.CMove(&f.B, &s, wasInverted)
	// newC = c' * t^-1
	s.Mul(&c, &t)
	f.C.CMove(&f.C, &s, wasInverted)
	return f, wasInverted
}

// Neg computes the field negation
func (f *fp6) Neg(arg *fp6) *fp6 {
	f.A.Neg(&arg.A)
	f.B.Neg(&arg.B)
	f.C.Neg(&arg.C)
	return f
}

// IsZero returns 1 if fp6 == 0, 0 otherwise
func (f *fp6) IsZero() int {
	return f.A.IsZero() & f.B.IsZero() & f.C.IsZero()
}

// IsOne returns 1 if fp6 == 1, 0 otherwise
func (f *fp6) IsOne() int {
	return f.A.IsOne() & f.B.IsZero() & f.B.IsZero()
}

// Equal returns 1 if fp6 == rhs, 0 otherwise
func (f *fp6) Equal(rhs *fp6) int {
	return f.A.Equal(&rhs.A) & f.B.Equal(&rhs.B) & f.C.Equal(&rhs.C)
}

// CMove performs conditional select.
// selects arg1 if choice == 0 and arg2 if choice == 1
func (f *fp6) CMove(arg1, arg2 *fp6, choice int) *fp6 {
	f.A.CMove(&arg1.A, &arg2.A, choice)
	f.B.CMove(&arg1.B, &arg2.B, choice)
	f.C.CMove(&arg1.C, &arg2.C, choice)
	return f
}
