package edwards25519

import (
	"fmt"
)

// Represents a point (s, t) on the Jacobi quartic associated to
// the Edwards curve
type JacobiPoint struct {
	S, T FieldElement
}

// Computes the at most 8 positive FieldElements f such that p == elligator2(f).
// Assumes p is even.
//
// Returns a bitmask of which elements in fes are set.
func (p *ExtendedPoint) RistrettoElligator2Inverse(fes *[8]FieldElement) uint8 {
	var setMask uint8
	var jcs [4]JacobiPoint
	var jc JacobiPoint

	// Elligator2 computes a Point from a FieldElement in two steps: first
	// it computes a (s,t) on the Jacobi quartic and then computes the
	// corresponding even point on the Edwards curve.
	//
	// We invert in three steps.  Any Ristretto point has four representatives
	// as even Edwards points.  For each of those even Edwards points,
	// there are two points on the Jacobi quartic that map to it.
	// Each of those eight points on the Jacobi quartic might have an
	// Elligator2 preimage.
	//
	// Essentially we first loop over the four representatives of our point,
	// then for each of them consider both points on the Jacobi quartic and
	// check whether they have an inverse under Elligator2.  We take the
	// following shortcut though.
	//
	//  We can compute two Jacobi quartic points for (x,y) and (-x,-y)
	//  at the same time.  The four Jacobi quartic points are two of
	//  such pairs.

	p.ToJacobiQuarticRistretto(&jcs)

	for j := 0; j < 4; j++ {
		setMask |= uint8(jcs[j].elligator2Inverse(&fes[2*j]) << uint(2*j))
		jc.Dual(&jcs[j])
		setMask |= uint8(jc.elligator2Inverse(&fes[2*j+1]) << uint(2*j+1))
	}

	return setMask
}

// Find a point on the Jacobi quartic associated to each of the four
// points Ristretto equivalent to p.
//
// There is one exception: for (0,-1) there is no point on the quartic and
// so we repeat one on the quartic equivalent to (0,1).
func (p *ExtendedPoint) ToJacobiQuarticRistretto(qs *[4]JacobiPoint) *ExtendedPoint {
	var X2, Y2, Z2, Y4, ZmY, ZpY, Z2mY2, gamma, den, sOverX, spOverXp, tmp FieldElement

	X2.Square(&p.X)     // X^2
	Y2.Square(&p.Y)     // Y^2
	Y4.Square(&Y2)      // Y^4
	Z2.Square(&p.Z)     // Z^2
	ZmY.sub(&p.Z, &p.Y) // Z - Y
	ZpY.add(&p.Z, &p.Y) // Z + Y
	Z2mY2.sub(&Z2, &Y2) // Z^2 - Y^2

	// gamma := 1/sqrt( Y^4 X^2 (Z^2 - Y^2) )
	gamma.Mul(&Y4, &X2)
	gamma.Mul(&gamma, &Z2mY2)
	gamma.InvSqrt(&gamma)

	// den := gamma * Y^2
	den.Mul(&gamma, &Y2)

	// sOverX := den * (Z - Y)
	// spOverXp := den * (Z + Y)
	sOverX.Mul(&den, &ZmY)
	spOverXp.Mul(&den, &ZpY)

	// s_0 := sOverX * X
	// s_1 := -spOverXp * X
	qs[0].S.Mul(&sOverX, &p.X)
	tmp.Mul(&spOverXp, &p.X)
	qs[1].S.Neg(&tmp)

	// t_0 := 2/sqrt(-d-1) * Z * sOverX
	// t_1 := 2/sqrt(-d-1) * Z * spOverXp
	tmp.Mul(&feDoubleInvSqrtMinusDMinusOne, &p.Z)
	qs[0].T.Mul(&tmp, &sOverX)
	qs[1].T.Mul(&tmp, &spOverXp)

	// den = 1/sqrt(1+d) (Y^2 - Z^2) gamma
	den.Neg(&Z2mY2)
	den.Mul(&den, &feInvSqrt1pD)
	den.Mul(&den, &gamma)

	// Same as before, but with the substitution (X, Y, Z) = (Y, X, i*Z)
	var iZ, iZmX, iZpX, sOverY, spOverYp FieldElement
	iZ.Mul(&feI, &p.Z)  // iZ
	iZmX.sub(&iZ, &p.X) // iZ - X
	iZpX.add(&iZ, &p.X) // iZ + X

	// sOverY := den * (iZ - Y)
	// spOverYp := den * (iZ + Y)
	sOverY.Mul(&den, &iZmX)
	spOverYp.Mul(&den, &iZpX)

	// s_2 := sOverY * Y
	// s_3 := -spOverYp * Y
	qs[2].S.Mul(&sOverY, &p.Y)
	tmp.Mul(&spOverYp, &p.Y)
	qs[3].S.Neg(&tmp)

	// t_2 := 2/sqrt(-d-1) * i*Z * sOverY
	// t_3 := 2/sqrt(-d-1) * i*Z * spOverYp
	tmp.Mul(&feDoubleInvSqrtMinusDMinusOne, &iZ)
	qs[2].T.Mul(&tmp, &sOverY)
	qs[3].T.Mul(&tmp, &spOverYp)

	// Special case: X=0 or Y=0.  Then return
	//
	//  (0,1)   (1,2i/sqrt(-d-1)   (-1,2i/sqrt(-d-1))
	//
	// Note that if X=0 or Y=0, then s_i = t_i = 0.
	XorYisZero := 1 - (p.X.IsNonZeroI() & p.Y.IsNonZeroI())
	qs[0].T.ConditionalSet(&feOne, XorYisZero)
	qs[1].T.ConditionalSet(&feOne, XorYisZero)
	qs[2].T.ConditionalSet(&feDoubleIInvSqrtMinusDMinusOne, XorYisZero)
	qs[3].T.ConditionalSet(&feDoubleIInvSqrtMinusDMinusOne, XorYisZero)
	qs[2].S.ConditionalSet(&feOne, XorYisZero)
	qs[3].S.ConditionalSet(&feMinusOne, XorYisZero)

	return p
}

func (p *JacobiPoint) Dual(q *JacobiPoint) *JacobiPoint {
	p.S.Neg(&q.S)
	p.T.Neg(&q.T)

	return p
}

// Elligator2 is defined in two steps: first a field element is converted
// to a point (s,t) on the Jacobi quartic associated to the Edwards curve.
// Then this point is mapped to a point on the Edwards curve.
// This function computes a field element that is mapped to a given (s,t)
// with Elligator2 if it exists.
//
// Returns 1 if a preimage is found and 0 if none exists.
func (p *JacobiPoint) elligator2Inverse(fe *FieldElement) int {
	var x, y, a, a2, S2, S4, invSqiY, negS2, out FieldElement

	// Special case: s = 0.  If s is zero, either t = 1 or t = -1.
	// If t=1, then sqrt(i*d) is the preimage.  Otherwise it's 0.

	sNonZero := p.S.IsNonZeroI()
	tEqualsOne := p.T.EqualsI(&feOne)
	out.Set(&feZero)
	out.ConditionalSet(&feSqrtID, tEqualsOne)

	ret := 1 - sNonZero
	done := 1 - sNonZero

	// a := (t+1) (d+1)/(d-1)
	a.add(&p.T, &feOne)
	a.Mul(&a, &feDp1OverDm1)
	a2.Square(&a)

	// y := 1/sqrt(i (s^4 - a^2)).
	S2.Square(&p.S)
	S4.Square(&S2)
	invSqiY.sub(&S4, &a2)

	// there is no preimage of the square root of i*(s^4-a^2) does not exist
	sq := y.InvSqrtI(&invSqiY)
	ret |= 1 - sq
	done |= sq

	// x := (a + sign(s)*s^2) y
	negS2.Neg(&S2)
	S2.ConditionalSet(&negS2, p.S.IsNegativeI())
	x.add(&a, &S2)
	x.Mul(&x, &y)

	// fe := abs(x)
	x.Abs(&x)
	out.ConditionalSet(&x, 1-done)
	fe.Set(&out)

	return int(ret)
}

// Set p to the point corresponding to the given point (s,t) on the
// associated Jacobi quartic.
func (p *CompletedPoint) SetJacobiQuartic(jc *JacobiPoint) *CompletedPoint {
	var s2 FieldElement
	s2.Square(&jc.S)

	// Set x to s * 2/sqrt(-d-1)
	p.X.Mul(&jc.S, &feDoubleInvSqrtMinusDMinusOne)

	// Set z to t
	p.Z.Set(&jc.T)

	// Set y to 1-s^2
	p.Y.sub(&feOne, &s2)

	// Set t to 1+s^2
	p.T.add(&feOne, &s2)
	return p
}

// Set p to the curvepoint corresponding to r0 via Mike Hamburg's variation
// on Elligator2 for Ristretto.  Returns p.
func (p *CompletedPoint) SetRistrettoElligator2(r0 *FieldElement) *CompletedPoint {
	var r, rPlusD, rPlusOne, D, N, ND, sqrt, twiddle, sgn FieldElement
	var rSubOne, r0i, sNeg FieldElement
	var jc JacobiPoint

	var b int32

	// r := i * r0^2
	r0i.Mul(r0, &feI)
	r.Mul(r0, &r0i)

	// D := -((d*r)+1) * (r + d)
	rPlusD.add(&feD, &r)
	D.Mul(&feD, &r)
	D.add(&D, &feOne)
	D.Mul(&D, &rPlusD)
	D.Neg(&D)

	// N := -(d^2 - 1)(r + 1)
	rPlusOne.add(&r, &feOne)
	N.Mul(&feOneMinusDSquared, &rPlusOne)

	// sqrt is the inverse square root of N*D or of i*N*D.
	// b=1 iff n1 is square.
	ND.Mul(&N, &D)

	b = sqrt.InvSqrtI(&ND)
	sqrt.Abs(&sqrt)

	twiddle.SetOne()
	twiddle.ConditionalSet(&r0i, 1-b)
	sgn.SetOne()
	sgn.ConditionalSet(&feMinusOne, 1-b)
	sqrt.Mul(&sqrt, &twiddle)

	// s = N * sqrt * twiddle
	jc.S.Mul(&sqrt, &N)

	// t = -sgn * sqrt * s * (r-1) * (d-1)^2 - 1
	jc.T.Neg(&sgn)
	jc.T.Mul(&sqrt, &jc.T)
	jc.T.Mul(&jc.S, &jc.T)
	jc.T.Mul(&feDMinusOneSquared, &jc.T)
	rSubOne.sub(&r, &feOne)
	jc.T.Mul(&rSubOne, &jc.T)
	jc.T.sub(&jc.T, &feOne)

	sNeg.Neg(&jc.S)
	jc.S.ConditionalSet(&sNeg, equal30(jc.S.IsNegativeI(), b))
	return p.SetJacobiQuartic(&jc)
}

// WARNING This operation is not constant-time.  Do not use for cryptography
//         unless you're sure this is not an issue.
func (p *JacobiPoint) String() string {
	return fmt.Sprintf("JacobiPoint(%v, %v)", p.S, p.T)
}
