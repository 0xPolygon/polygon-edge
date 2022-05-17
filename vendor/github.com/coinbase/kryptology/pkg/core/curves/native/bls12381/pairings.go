package bls12381

const coefficientsG2 = 68

type Engine struct {
	pairs []pair
}

type pair struct {
	g1 G1
	g2 G2
}

type g2Prepared struct {
	identity     int
	coefficients []coefficients
}

type coefficients struct {
	a, b, c fp2
}

func (c *coefficients) CMove(arg1, arg2 *coefficients, choice int) *coefficients {
	c.a.CMove(&arg1.a, &arg2.a, choice)
	c.b.CMove(&arg1.b, &arg2.b, choice)
	c.c.CMove(&arg1.c, &arg2.c, choice)
	return c
}

// AddPair adds a pair of points to be paired
func (e *Engine) AddPair(g1 *G1, g2 *G2) *Engine {
	var p pair
	p.g1.ToAffine(g1)
	p.g2.ToAffine(g2)
	if p.g1.IsIdentity()|p.g2.IsIdentity() == 0 {
		e.pairs = append(e.pairs, p)
	}
	return e
}

// AddPairInvG1 adds a pair of points to be paired. G1 point is negated
func (e *Engine) AddPairInvG1(g1 *G1, g2 *G2) *Engine {
	var p G1
	p.Neg(g1)
	return e.AddPair(&p, g2)
}

// AddPairInvG2 adds a pair of points to be paired. G2 point is negated
func (e *Engine) AddPairInvG2(g1 *G1, g2 *G2) *Engine {
	var p G2
	p.Neg(g2)
	return e.AddPair(g1, &p)
}

func (e *Engine) Reset() *Engine {
	e.pairs = []pair{}
	return e
}

func (e *Engine) Check() bool {
	return e.pairing().IsOne() == 1
}

func (e *Engine) Result() *Gt {
	return e.pairing()
}

func (e *Engine) pairing() *Gt {
	f := new(Gt).SetOne()
	if len(e.pairs) == 0 {
		return f
	}
	coeffs := e.computeCoeffs()
	e.millerLoop((*fp12)(f), coeffs)
	return f.FinalExponentiation(f)
}

func (e *Engine) millerLoop(f *fp12, coeffs []g2Prepared) {
	newF := new(fp12).SetZero()
	found := 0
	cIdx := 0
	for i := 63; i >= 0; i-- {
		x := int(((paramX >> 1) >> i) & 1)
		if found == 0 {
			found |= x
			continue
		}

		// doubling
		for j, terms := range coeffs {
			identity := e.pairs[j].g1.IsIdentity() | terms.identity
			newF.Set(f)
			ell(newF, terms.coefficients[cIdx], &e.pairs[j].g1)
			f.CMove(newF, f, identity)
		}
		cIdx++

		if x == 1 {
			// adding
			for j, terms := range coeffs {
				identity := e.pairs[j].g1.IsIdentity() | terms.identity
				newF.Set(f)
				ell(newF, terms.coefficients[cIdx], &e.pairs[j].g1)
				f.CMove(newF, f, identity)
			}
			cIdx++
		}
		f.Square(f)
	}
	for j, terms := range coeffs {
		identity := e.pairs[j].g1.IsIdentity() | terms.identity
		newF.Set(f)
		ell(newF, terms.coefficients[cIdx], &e.pairs[j].g1)
		f.CMove(newF, f, identity)
	}
	f.Conjugate(f)
}

func (e *Engine) computeCoeffs() []g2Prepared {
	coeffs := make([]g2Prepared, len(e.pairs))
	for i, p := range e.pairs {
		identity := p.g2.IsIdentity()
		q := new(G2).Generator()
		q.CMove(&p.g2, q, identity)
		c := new(G2).Set(q)
		cfs := make([]coefficients, coefficientsG2)
		found := 0
		k := 0

		for j := 63; j >= 0; j-- {
			x := int(((paramX >> 1) >> j) & 1)
			if found == 0 {
				found |= x
				continue
			}
			cfs[k] = doublingStep(c)
			k++

			if x == 1 {
				cfs[k] = additionStep(c, q)
				k++
			}
		}
		cfs[k] = doublingStep(c)
		coeffs[i] = g2Prepared{
			coefficients: cfs, identity: identity,
		}
	}
	return coeffs
}

func ell(f *fp12, coeffs coefficients, p *G1) {
	var x, y fp2
	x.A.Mul(&coeffs.a.A, &p.y)
	x.B.Mul(&coeffs.a.B, &p.y)
	y.A.Mul(&coeffs.b.A, &p.x)
	y.B.Mul(&coeffs.b.B, &p.x)
	f.MulByABD(f, &coeffs.c, &y, &x)
}

func doublingStep(p *G2) coefficients {
	// Adaptation of Algorithm 26, https://eprint.iacr.org/2010/354.pdf
	var t0, t1, t2, t3, t4, t5, t6, zsqr fp2
	t0.Square(&p.x)
	t1.Square(&p.y)
	t2.Square(&t1)
	t3.Add(&t1, &p.x)
	t3.Square(&t3)
	t3.Sub(&t3, &t0)
	t3.Sub(&t3, &t2)
	t3.Double(&t3)
	t4.Double(&t0)
	t4.Add(&t4, &t0)
	t6.Add(&p.x, &t4)
	t5.Square(&t4)
	zsqr.Square(&p.z)
	p.x.Sub(&t5, &t3)
	p.x.Sub(&p.x, &t3)
	p.z.Add(&p.z, &p.y)
	p.z.Square(&p.z)
	p.z.Sub(&p.z, &t1)
	p.z.Sub(&p.z, &zsqr)
	p.y.Sub(&t3, &p.x)
	p.y.Mul(&p.y, &t4)
	t2.Double(&t2)
	t2.Double(&t2)
	t2.Double(&t2)
	p.y.Sub(&p.y, &t2)
	t3.Mul(&t4, &zsqr)
	t3.Double(&t3)
	t3.Neg(&t3)
	t6.Square(&t6)
	t6.Sub(&t6, &t0)
	t6.Sub(&t6, &t5)
	t1.Double(&t1)
	t1.Double(&t1)
	t6.Sub(&t6, &t1)
	t0.Mul(&p.z, &zsqr)
	t0.Double(&t0)

	return coefficients{
		a: t0, b: t3, c: t6,
	}
}

func additionStep(r, q *G2) coefficients {
	// Adaptation of Algorithm 27, https://eprint.iacr.org/2010/354.pdf
	var zsqr, ysqr fp2
	var t0, t1, t2, t3, t4, t5, t6, t7, t8, t9, t10 fp2
	zsqr.Square(&r.z)
	ysqr.Square(&q.y)
	t0.Mul(&zsqr, &q.x)
	t1.Add(&q.y, &r.z)
	t1.Square(&t1)
	t1.Sub(&t1, &ysqr)
	t1.Sub(&t1, &zsqr)
	t1.Mul(&t1, &zsqr)
	t2.Sub(&t0, &r.x)
	t3.Square(&t2)
	t4.Double(&t3)
	t4.Double(&t4)
	t5.Mul(&t4, &t2)
	t6.Sub(&t1, &r.y)
	t6.Sub(&t6, &r.y)
	t9.Mul(&t6, &q.x)
	t7.Mul(&t4, &r.x)
	r.x.Square(&t6)
	r.x.Sub(&r.x, &t5)
	r.x.Sub(&r.x, &t7)
	r.x.Sub(&r.x, &t7)
	r.z.Add(&r.z, &t2)
	r.z.Square(&r.z)
	r.z.Sub(&r.z, &zsqr)
	r.z.Sub(&r.z, &t3)
	t10.Add(&q.y, &r.z)
	t8.Sub(&t7, &r.x)
	t8.Mul(&t8, &t6)
	t0.Mul(&r.y, &t5)
	t0.Double(&t0)
	r.y.Sub(&t8, &t0)
	t10.Square(&t10)
	t10.Sub(&t10, &ysqr)
	zsqr.Square(&r.z)
	t10.Sub(&t10, &zsqr)
	t9.Double(&t9)
	t9.Sub(&t9, &t10)
	t10.Double(&r.z)
	t6.Neg(&t6)
	t1.Double(&t6)

	return coefficients{
		a: t10, b: t1, c: t9,
	}
}
