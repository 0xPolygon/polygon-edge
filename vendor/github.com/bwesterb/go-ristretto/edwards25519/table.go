package edwards25519

type NielsPoint struct {
	YPlusX, YMinusX, XY2D FieldElement
}

// Precomputed scalar multiplication table
type ScalarMultTable [32][8]NielsPoint

// Set p to zero, the neutral element.  Return p.
func (p *NielsPoint) SetZero() *NielsPoint {
	p.YMinusX.SetOne()
	p.YPlusX.SetOne()
	p.XY2D.SetZero()
	return p
}

// Set p to q.  Returns p.
func (p *NielsPoint) Set(q *NielsPoint) *NielsPoint {
	p.YPlusX.Set(&q.YPlusX)
	p.YMinusX.Set(&q.YMinusX)
	p.XY2D.Set(&q.XY2D)
	return p
}

// Set p to q if b == 1.  Assumes b is 0 or 1.  Returns p.
func (p *NielsPoint) ConditionalSet(q *NielsPoint, b int32) *NielsPoint {
	p.YPlusX.ConditionalSet(&q.YPlusX, b)
	p.YMinusX.ConditionalSet(&q.YMinusX, b)
	p.XY2D.ConditionalSet(&q.XY2D, b)
	return p
}

// Set p to -q.  Returns p.
func (p *NielsPoint) Neg(q *NielsPoint) *NielsPoint {
	p.YMinusX.Set(&q.YPlusX)
	p.YPlusX.Set(&q.YMinusX)
	p.XY2D.Neg(&q.XY2D)
	return p
}

// Sets p to q+r.  Returns p.
func (p *CompletedPoint) AddExtendedNiels(q *ExtendedPoint, r *NielsPoint) *CompletedPoint {
	var t0 FieldElement

	p.X.add(&q.Y, &q.X)
	p.Y.sub(&q.Y, &q.X)
	p.Z.Mul(&p.X, &r.YPlusX)
	p.Y.Mul(&p.Y, &r.YMinusX)
	p.T.Mul(&r.XY2D, &q.T)
	t0.add(&q.Z, &q.Z)
	p.X.sub(&p.Z, &p.Y)
	p.Y.add(&p.Z, &p.Y)
	p.Z.add(&t0, &p.T)
	p.T.sub(&t0, &p.T)
	return p
}

// Set p to q-r.  Returns p.
func (p *CompletedPoint) SubExtendedNiels(q *ExtendedPoint, r *NielsPoint) *CompletedPoint {
	var t0 FieldElement

	p.X.add(&q.Y, &q.X)
	p.Y.sub(&q.Y, &q.X)
	p.Z.Mul(&p.X, &r.YMinusX)
	p.Y.Mul(&p.Y, &r.YPlusX)
	p.T.Mul(&r.XY2D, &q.T)
	t0.add(&q.Z, &q.Z)
	p.X.sub(&p.Z, &p.Y)
	p.Y.add(&p.Z, &p.Y)
	p.Z.sub(&t0, &p.T)
	p.T.add(&t0, &p.T)
	return p
}

// Sets p to q.  Returns p.
func (p *NielsPoint) SetExtended(q *ExtendedPoint) *NielsPoint {
	var x, y, zInv FieldElement
	zInv.Inverse(&q.Z)
	x.Mul(&q.X, &zInv)
	y.Mul(&q.Y, &zInv)
	p.YPlusX.Add(&y, &x)
	p.YMinusX.Sub(&y, &x)
	p.XY2D.Mul(&x, &y)
	p.XY2D.Add(&p.XY2D, &p.XY2D)
	p.XY2D.Mul(&p.XY2D, &feD)
	return p
}

// Fill the table t with data for the point p.
func (t *ScalarMultTable) Compute(p *ExtendedPoint) {
	var c, cp ExtendedPoint
	var c_pp ProjectivePoint
	var c_cp CompletedPoint
	cp.Set(p)
	for i := 0; i < 32; i++ {
		c.SetZero()
		for v := 0; v < 8; v++ {
			c.Add(&c, &cp)
			t[i][v].SetExtended(&c)
		}
		c_cp.DoubleExtended(&c)
		c_pp.SetCompleted(&c_cp)
		c_cp.DoubleProjective(&c_pp)
		c_pp.SetCompleted(&c_cp)
		c_cp.DoubleProjective(&c_pp)
		c_pp.SetCompleted(&c_cp)
		c_cp.DoubleProjective(&c_pp)
		c_pp.SetCompleted(&c_cp)
		c_cp.DoubleProjective(&c_pp)
		cp.SetCompleted(&c_cp)
	}
}

// Compute 4-bit signed window for the scalar s
func computeScalarWindow4(s *[32]byte, w *[64]int8) {
	for i := 0; i < 32; i++ {
		w[2*i] = int8(s[i] & 15)
		w[2*i+1] = int8((s[i] >> 4) & 15)
	}
	carry := int8(0)
	for i := 0; i < 63; i++ {
		w[i] += carry
		carry = (w[i] + 8) >> 4
		w[i] -= carry << 4
	}
	w[63] += carry
}

// Set p to s * q, where t was computed for q using t.Compute(q).
func (t *ScalarMultTable) ScalarMult(p *ExtendedPoint, s *[32]byte) {
	var w [64]int8
	computeScalarWindow4(s, &w)

	p.SetZero()
	var np NielsPoint
	var cp CompletedPoint
	var pp ProjectivePoint

	for i := int32(0); i < 32; i++ {
		t.selectPoint(&np, i, int32(w[2*i+1]))
		cp.AddExtendedNiels(p, &np)
		p.SetCompleted(&cp)
	}

	cp.DoubleExtended(p)
	pp.SetCompleted(&cp)
	cp.DoubleProjective(&pp)
	pp.SetCompleted(&cp)
	cp.DoubleProjective(&pp)
	pp.SetCompleted(&cp)
	cp.DoubleProjective(&pp)
	p.SetCompleted(&cp)

	for i := int32(0); i < 32; i++ {
		t.selectPoint(&np, i, int32(w[2*i]))
		cp.AddExtendedNiels(p, &np)
		p.SetCompleted(&cp)
	}
}

func (t *ScalarMultTable) VarTimeScalarMult(p *ExtendedPoint, s *[32]byte) {
	var w [64]int8
	computeScalarWindow4(s, &w)

	p.SetZero()
	var np NielsPoint
	var cp CompletedPoint
	var pp ProjectivePoint

	for i := int32(0); i < 32; i++ {
		if t.varTimeSelectPoint(&np, i, int32(w[2*i+1])) {
			cp.AddExtendedNiels(p, &np)
			p.SetCompleted(&cp)
		}
	}

	cp.DoubleExtended(p)
	pp.SetCompleted(&cp)
	cp.DoubleProjective(&pp)
	pp.SetCompleted(&cp)
	cp.DoubleProjective(&pp)
	pp.SetCompleted(&cp)
	cp.DoubleProjective(&pp)
	p.SetCompleted(&cp)

	for i := int32(0); i < 32; i++ {
		if t.varTimeSelectPoint(&np, i, int32(w[2*i])) {
			cp.AddExtendedNiels(p, &np)
			p.SetCompleted(&cp)
		}
	}
}

func (t *ScalarMultTable) selectPoint(p *NielsPoint, pos int32, b int32) {
	bNegative := negative(b)
	bAbs := b - (((-bNegative) & b) << 1)
	p.SetZero()
	for i := int32(0); i < 8; i++ {
		p.ConditionalSet(&t[pos][i], equal30(bAbs, i+1))
	}
	var negP NielsPoint
	negP.Neg(p)
	p.ConditionalSet(&negP, bNegative)
}

func (t *ScalarMultTable) varTimeSelectPoint(p *NielsPoint, pos int32, b int32) bool {
	if b == 0 {
		return false
	}
	if b < 0 {
		p.Neg(&t[pos][-b-1])
	} else {
		p.Set(&t[pos][b-1])
	}
	return true
}
