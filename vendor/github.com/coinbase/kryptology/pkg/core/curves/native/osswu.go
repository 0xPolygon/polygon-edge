package native

// SswuParams for computing the Simplified SWU mapping
// for hash to curve implementations
type SswuParams struct {
	C1, C2, A, B, Z [FieldLimbs]uint64
}

// Osswu3mod4 computes the simplified map optmized for 3 mod 4 primes
// https://tools.ietf.org/html/draft-irtf-cfrg-hash-to-curve-11#appendix-G.2.1
func (p *SswuParams) Osswu3mod4(u *Field) (x, y *Field) {
	var tv1, tv2, tv3, tv4, xd, x1n, x2n, gxd, gx1, aNeg, zA, y1, y2 [FieldLimbs]uint64
	var wasInverted int
	u.Arithmetic.Mul(&tv1, &u.Value, &u.Value) // tv1 = u^2
	u.Arithmetic.Mul(&tv3, &p.Z, &tv1)         // tv3 = z * tv1
	u.Arithmetic.Square(&tv2, &tv3)            // tv2 = tv3^2
	u.Arithmetic.Add(&xd, &tv2, &tv3)          // xd = tv2 + tv3
	u.Arithmetic.Add(&x1n, &u.Params.R, &xd)   // x1n = (xd + 1)
	u.Arithmetic.Mul(&x1n, &x1n, &p.B)         // x1n * B
	u.Arithmetic.Neg(&aNeg, &p.A)
	u.Arithmetic.Mul(&xd, &xd, &aNeg) // xd = -A * xd

	xdIsZero := (&Field{
		Value: xd,
	}).IsZero()
	u.Arithmetic.Mul(&zA, &p.Z, &p.A)
	u.Arithmetic.Selectznz(&xd, &xd, &zA, xdIsZero) // xd = z * A if xd == 0

	u.Arithmetic.Square(&tv2, &xd)     // tv2 = xd^2
	u.Arithmetic.Mul(&gxd, &tv2, &xd)  // gxd = tv2 * xd
	u.Arithmetic.Mul(&tv2, &tv2, &p.A) // tv2 = A * tv2

	u.Arithmetic.Square(&gx1, &x1n)    // gx1 = x1n^2
	u.Arithmetic.Add(&gx1, &gx1, &tv2) // gx1 = gx1 + tv2
	u.Arithmetic.Mul(&gx1, &gx1, &x1n) // gx1 = gx1 * x1n
	u.Arithmetic.Mul(&tv2, &gxd, &p.B) // tv2 = B * gxd
	u.Arithmetic.Add(&gx1, &gx1, &tv2) // gx1 = gx1 + tv2

	u.Arithmetic.Square(&tv4, &gxd)    // tv4 = gxd^2
	u.Arithmetic.Mul(&tv2, &gx1, &gxd) // tv2 = gx1 * gxd
	u.Arithmetic.Mul(&tv4, &tv4, &tv2) // tv4 = tv4 * tv2

	Pow(&y1, &tv4, &p.C1, u.Params, u.Arithmetic) // y1 = tv4^C1
	u.Arithmetic.Mul(&y1, &y1, &tv2)              //y1 = y1 * tv2
	u.Arithmetic.Mul(&x2n, &tv3, &x1n)            // x2n = tv3 * x1n

	u.Arithmetic.Mul(&y2, &y1, &p.C2)    // y2 = y1 * c2
	u.Arithmetic.Mul(&y2, &y2, &tv1)     // y2 = y2 * tv1
	u.Arithmetic.Mul(&y2, &y2, &u.Value) // y2 = y2 * u

	u.Arithmetic.Square(&tv2, &y1)     // tv2 = y1^2
	u.Arithmetic.Mul(&tv2, &tv2, &gxd) // tv2 = tv2 * gxd

	e2 := (&Field{Value: tv2}).Equal(&Field{Value: gx1})

	x = new(Field).Set(u)
	y = new(Field).Set(u)

	// If e2, x = x1, else x = x2
	u.Arithmetic.Selectznz(&x.Value, &x2n, &x1n, e2)

	// xn / xd
	u.Arithmetic.Invert(&wasInverted, &tv1, &xd)
	u.Arithmetic.Mul(&tv1, &x.Value, &tv1)
	u.Arithmetic.Selectznz(&x.Value, &x.Value, &tv1, wasInverted)

	// If e2, y = y1, else y = y2
	u.Arithmetic.Selectznz(&y.Value, &y2, &y1, e2)

	uBytes := u.Bytes()
	yBytes := y.Bytes()

	usign := uBytes[0] & 1
	ysign := yBytes[0] & 1

	// Fix sign of y
	if usign != ysign {
		y.Neg(y)
	}

	return
}
