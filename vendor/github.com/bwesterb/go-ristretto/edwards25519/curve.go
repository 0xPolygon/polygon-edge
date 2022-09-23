// Go implementation of the elliptic curve Edwards25519 of which the
// Ristretto group is a subquotient.
package edwards25519

import (
	"crypto/subtle"
	"encoding/hex"
	"fmt"
)

// (X:Y:Z:T) satisfying x=X/Z, y=Y/Z, X*Y=Z*T.  Aka P3.
type ExtendedPoint struct {
	X, Y, Z, T FieldElement
}

// ((X:Z),(Y:T)) satisfying x=X/Z, y=Y/T. Aka P1P1.
type CompletedPoint struct {
	X, Y, Z, T FieldElement
}

// (X:Y:Z) satisfying x=X/Z, y=Y/Z.
type ProjectivePoint struct {
	X, Y, Z FieldElement
}

// Set p to (-i,0), a point Ristretto-equivalent to 0.  Returns p.
func (p *ExtendedPoint) SetTorsion3() *ExtendedPoint {
	p.X.Set(&feMinusI)
	p.Y.SetZero()
	p.Z.Set(&feOne)
	p.T.SetZero()
	return p
}

// Set p to (i,0), a point Ristretto-equivalent to 0.  Returns p.
func (p *ExtendedPoint) SetTorsion2() *ExtendedPoint {
	p.X.Set(&feI)
	p.Y.SetZero()
	p.Z.Set(&feOne)
	p.T.SetZero()
	return p
}

// Set p to (0,-1), a point Ristretto-equivalent to 0.  Returns p.
func (p *ExtendedPoint) SetTorsion1() *ExtendedPoint {
	p.X.SetZero()
	p.Y.Set(&feMinusOne)
	p.Z.Set(&feOne)
	p.T.SetZero()
	return p
}

// Set p to zero, the neutral element.  Return p.
func (p *ProjectivePoint) SetZero() *ProjectivePoint {
	p.X.SetZero()
	p.Y.SetOne()
	p.Z.SetOne()
	return p
}

// Set p to zero, the neutral element.  Return p.
func (p *ExtendedPoint) SetZero() *ExtendedPoint {
	p.X.SetZero()
	p.Y.SetOne()
	p.Z.SetOne()
	p.T.SetZero()
	return p
}

// Set p to the basepoint (x,4/5) with x>=0.  Returns p
func (p *ExtendedPoint) SetBase() *ExtendedPoint {
	return p.Set(&epBase)
}

// Set p to q.  Returns p.
func (p *ExtendedPoint) Set(q *ExtendedPoint) *ExtendedPoint {
	p.X.Set(&q.X)
	p.Y.Set(&q.Y)
	p.Z.Set(&q.Z)
	p.T.Set(&q.T)
	return p
}

// Set p to q if b == 1.  Assumes b is 0 or 1.   Returns p.
func (p *ExtendedPoint) ConditionalSet(q *ExtendedPoint, b int32) *ExtendedPoint {
	p.X.ConditionalSet(&q.X, b)
	p.Y.ConditionalSet(&q.Y, b)
	p.Z.ConditionalSet(&q.Z, b)
	p.T.ConditionalSet(&q.T, b)
	return p
}

// Sets p to q+r.  Returns p
func (p *CompletedPoint) AddExtended(q, r *ExtendedPoint) *CompletedPoint {
	var a, b, c, d, t FieldElement

	a.sub(&q.Y, &q.X)
	t.sub(&r.Y, &r.X)
	a.Mul(&a, &t)
	b.add(&q.X, &q.Y)
	t.add(&r.X, &r.Y)
	b.Mul(&b, &t)
	c.Mul(&q.T, &r.T)
	c.Mul(&c, &fe2D)
	d.Mul(&q.Z, &r.Z)
	d.add(&d, &d)
	p.X.sub(&b, &a)
	p.T.sub(&d, &c)
	p.Z.add(&d, &c)
	p.Y.add(&b, &a)

	return p
}

// Sets p to q-r.  Returns p
func (p *CompletedPoint) SubExtended(q, r *ExtendedPoint) *CompletedPoint {
	var a, b, c, d, t FieldElement

	a.sub(&q.Y, &q.X)
	t.add(&r.Y, &r.X)
	a.Mul(&a, &t)
	b.add(&q.X, &q.Y)
	t.sub(&r.Y, &r.X)
	b.Mul(&b, &t)
	c.Mul(&q.T, &r.T)
	c.Mul(&c, &fe2D)
	d.Mul(&q.Z, &r.Z)
	d.add(&d, &d)
	p.X.sub(&b, &a)
	p.T.add(&d, &c)
	p.Z.sub(&d, &c)
	p.Y.add(&b, &a)

	return p
}

// Set p to 2 * q.  Returns p.
func (p *CompletedPoint) DoubleProjective(q *ProjectivePoint) *CompletedPoint {
	var t0 FieldElement

	p.X.Square(&q.X)
	p.Z.Square(&q.Y)
	p.T.DoubledSquare(&q.Z)
	p.Y.add(&q.X, &q.Y)
	t0.Square(&p.Y)
	p.Y.add(&p.Z, &p.X)
	p.Z.sub(&p.Z, &p.X)
	p.X.sub(&t0, &p.Y)
	p.T.sub(&p.T, &p.Z)

	return p
}

// Set p to 2 * q.  Returns p.
func (p *CompletedPoint) DoubleExtended(q *ExtendedPoint) *CompletedPoint {
	var a, b, c, d FieldElement

	a.Square(&q.X)
	b.Square(&q.Y)
	c.DoubledSquare(&q.Z)
	d.Neg(&a)
	p.X.add(&q.X, &q.Y)
	p.X.Square(&p.X)
	p.X.sub(&p.X, &a)
	p.X.sub(&p.X, &b)
	p.Z.add(&d, &b)
	p.T.sub(&p.Z, &c)
	p.Y.sub(&d, &b)

	return p
}

// Set p to q.  Returns p.
func (p *ProjectivePoint) SetExtended(q *ExtendedPoint) *ProjectivePoint {
	p.X.Set(&q.X)
	p.Y.Set(&q.Y)
	p.Z.Set(&q.Z)
	return p
}

// Set p to q.  Returns p.
func (p *ProjectivePoint) SetCompleted(q *CompletedPoint) *ProjectivePoint {
	p.X.Mul(&q.X, &q.T)
	p.Y.Mul(&q.Y, &q.Z)
	p.Z.Mul(&q.Z, &q.T)
	return p
}

// Set p to 2 * q. Returns p.
func (p *ExtendedPoint) Double(q *ExtendedPoint) *ExtendedPoint {
	var tmp CompletedPoint
	tmp.DoubleExtended(q)
	p.SetCompleted(&tmp)
	return p
}

// Set p to q + r. Returns p.
func (p *ExtendedPoint) Add(q, r *ExtendedPoint) *ExtendedPoint {
	var tmp CompletedPoint
	tmp.AddExtended(q, r)
	p.SetCompleted(&tmp)
	return p
}

// Set p to q - r. Returns p.
func (p *ExtendedPoint) Sub(q, r *ExtendedPoint) *ExtendedPoint {
	var tmp CompletedPoint
	tmp.SubExtended(q, r)
	p.SetCompleted(&tmp)
	return p
}

// Sets p to q.  Returns p.
func (p *ExtendedPoint) SetCompleted(q *CompletedPoint) *ExtendedPoint {
	p.X.Mul(&q.X, &q.T)
	p.Y.Mul(&q.Y, &q.Z)
	p.Z.Mul(&q.Z, &q.T)
	p.T.Mul(&q.X, &q.Y)
	return p
}

// Set p to a point corresponding to the encoded group element of
// the ristretto group.  Returns whether the buffer encoded a group element.
func (p *ExtendedPoint) SetRistretto(buf *[32]byte) bool {
	var s, s2, chk, yDen, yNum, yDen2, xDen2, isr, xDenInv FieldElement
	var yDenInv, t FieldElement
	var b, ret int32
	var buf2 [32]byte

	s.SetBytes(buf)

	// ensures 0 ≤ s < 2^255-19
	s.BytesInto(&buf2)
	ret = int32(1 - subtle.ConstantTimeCompare(buf[:], buf2[:]))
	ret |= int32(buf2[0] & 1) // ensure s is positive

	s2.Square(&s)
	yDen.add(&feOne, &s2)
	yNum.sub(&feOne, &s2)
	yDen2.Square(&yDen)
	xDen2.Square(&yNum)
	xDen2.Mul(&xDen2, &feD)
	xDen2.add(&xDen2, &yDen2)
	xDen2.Neg(&xDen2)
	t.Mul(&xDen2, &yDen2)
	isr.InvSqrt(&t)
	chk.Square(&isr)
	chk.Mul(&chk, &t)
	ret |= 1 - chk.IsOneI()
	xDenInv.Mul(&isr, &yDen)
	yDenInv.Mul(&xDenInv, &isr)
	yDenInv.Mul(&yDenInv, &xDen2)
	p.X.Mul(&s, &xDenInv)
	p.X.add(&p.X, &p.X)
	b = p.X.IsNegativeI()
	t.Neg(&p.X)
	p.X.ConditionalSet(&t, b)
	p.Y.Mul(&yNum, &yDenInv)
	p.Z.SetOne()
	p.T.Mul(&p.X, &p.Y)
	ret |= p.T.IsNegativeI()
	ret |= 1 - p.Y.IsNonZeroI()
	p.X.ConditionalSet(&feZero, ret)
	p.Y.ConditionalSet(&feZero, ret)
	p.Z.ConditionalSet(&feZero, ret)
	p.T.ConditionalSet(&feZero, ret)
	return ret == 0
}

// Pack p using the Ristretto encoding and return it.
// Requires p to be even.
func (p *ExtendedPoint) Ristretto() []byte {
	var buf [32]byte
	p.RistrettoInto(&buf)
	return buf[:]
}

// Pack p using the Ristretto encoding and write to buf.  Returns p.
// Requires p to be even.
func (p *ExtendedPoint) RistrettoInto(buf *[32]byte) *ExtendedPoint {
	var d, u1, u2, isr, i1, i2, zInv, denInv, nx, ny, s FieldElement
	var b int32

	d.add(&p.Z, &p.Y)
	u1.sub(&p.Z, &p.Y)
	u1.Mul(&u1, &d)

	u2.Mul(&p.X, &p.Y)

	isr.Square(&u2)
	isr.Mul(&isr, &u1)
	isr.InvSqrt(&isr)

	i1.Mul(&isr, &u1)
	i2.Mul(&isr, &u2)

	zInv.Mul(&i1, &i2)
	zInv.Mul(&zInv, &p.T)

	d.Mul(&zInv, &p.T)

	nx.Mul(&p.Y, &feI)
	ny.Mul(&p.X, &feI)
	denInv.Mul(&feInvSqrtMinusDMinusOne, &i1)

	b = 1 - d.IsNegativeI()
	nx.ConditionalSet(&p.X, b)
	ny.ConditionalSet(&p.Y, b)
	denInv.ConditionalSet(&i2, b)

	d.Mul(&nx, &zInv)
	b = d.IsNegativeI()
	d.Neg(&ny)
	ny.ConditionalSet(&d, b)

	s.sub(&p.Z, &ny)
	s.Mul(&s, &denInv)

	b = s.IsNegativeI()
	d.Neg(&s)
	s.ConditionalSet(&d, b)

	s.BytesInto(buf)
	return p
}

// Compute 5-bit window for the scalar s.
func computeScalarWindow5(s *[32]byte, w *[51]int8) {
	for i := 0; i < 6; i++ {
		w[8*i+0] = int8(s[5*i+0] & 31)
		w[8*i+1] = int8((s[5*i+0] >> 5) & 31)
		w[8*i+1] ^= int8((s[5*i+1] << 3) & 31)
		w[8*i+2] = int8((s[5*i+1] >> 2) & 31)
		w[8*i+3] = int8((s[5*i+1] >> 7) & 31)
		w[8*i+3] ^= int8((s[5*i+2] << 1) & 31)
		w[8*i+4] = int8((s[5*i+2] >> 4) & 31)
		w[8*i+4] ^= int8((s[5*i+3] << 4) & 31)
		w[8*i+5] = int8((s[5*i+3] >> 1) & 31)
		w[8*i+6] = int8((s[5*i+3] >> 6) & 31)
		w[8*i+6] ^= int8((s[5*i+4] << 2) & 31)
		w[8*i+7] = int8((s[5*i+4] >> 3) & 31)
	}
	w[8*6+0] = int8(s[5*6+0] & 31)
	w[8*6+1] = int8((s[5*6+0] >> 5) & 31)
	w[8*6+1] ^= int8((s[5*6+1] << 3) & 31)
	w[8*6+2] = int8((s[5*6+1] >> 2) & 31)

	/* Making it signed */
	var carry int8 = 0
	for i := 0; i < 50; i++ {
		w[i] += carry
		w[i+1] += w[i] >> 5
		w[i] &= 31
		carry = w[i] >> 4
		w[i] -= carry << 5
	}
	w[50] += carry
}

// Set p to s * q.  Returns p.
func (p *ExtendedPoint) ScalarMult(q *ExtendedPoint, s *[32]byte) *ExtendedPoint {
	// See eg. https://cryptojedi.org/peter/data/eccss-20130911b.pdf
	var lut [17]ExtendedPoint
	var t ExtendedPoint
	var window [51]int8

	// Precomputations.
	computeScalarWindow5(s, &window)
	lut[0].SetZero()
	lut[1].Set(q)
	for i := 2; i < 16; i += 2 {
		lut[i].Double(&lut[i>>1])
		lut[i+1].Add(&lut[i], q)
	}
	lut[16].Double(&lut[8])

	// Compute!
	p.SetZero()
	for i := 50; i >= 0; i-- {
		var pp ProjectivePoint
		var cp CompletedPoint
		cp.DoubleExtended(p)
		for z := 0; z < 4; z++ {
			pp.SetCompleted(&cp)
			cp.DoubleProjective(&pp)
		}
		p.SetCompleted(&cp)

		t.Set(&lut[0])
		b := int32(window[i])
		for j := 1; j <= 16; j++ {
			c := equal15(b, int32(-j)) | equal15(b, int32(j))
			t.ConditionalSet(&lut[j], c)
		}
		var v FieldElement
		c := negative(b)
		v.Neg(&t.X)
		t.X.ConditionalSet(&v, c)
		v.Neg(&t.T)
		t.T.ConditionalSet(&v, c)

		p.Add(p, &t)
	}

	return p
}

// Sets p to -q.  Returns p.
func (p *ExtendedPoint) Neg(q *ExtendedPoint) *ExtendedPoint {
	p.X.Neg(&q.X)
	p.Y.Set(&q.Y)
	p.Z.Set(&q.Z)
	p.T.Neg(&q.T)
	return p
}

// Returns 1 if p and q are in the same Ristretto equivalence class.
// Assumes p and q are both even.
func (p *ExtendedPoint) RistrettoEqualsI(q *ExtendedPoint) int32 {
	var x1y2, x2y1, x1x2, y1y2 FieldElement
	x1y2.Mul(&p.X, &q.Y)
	x2y1.Mul(&q.X, &p.Y)
	x1x2.Mul(&p.X, &q.X)
	y1y2.Mul(&p.Y, &q.Y)
	return 1 - ((1 - x1y2.EqualsI(&x2y1)) & (1 - x1x2.EqualsI(&y1y2)))
}

// WARNING This operation is not constant-time.  Do not use for cryptography
//         unless you're sure this is not an issue.
func (p *ExtendedPoint) String() string {
	return fmt.Sprintf("ExtendedPoint(%v, %v, %v, %v; %v)",
		p.X, p.Y, p.Z, p.T, hex.EncodeToString(p.Ristretto()))
}

// WARNING This operation is not constant-time.  Do not use for cryptography
//         unless you're sure this is not an issue.
func (p *CompletedPoint) String() string {
	var ep ExtendedPoint
	ep.SetCompleted(p)
	return fmt.Sprintf("CompletedPoint(%v, %v, %v, %v; %v)",
		p.X, p.Y, p.Z, p.T, hex.EncodeToString(ep.Ristretto()))
}
