package native

// IsogenyParams are the parameters needed to map from an isogeny to the main curve
type IsogenyParams struct {
	XNum [][FieldLimbs]uint64
	XDen [][FieldLimbs]uint64
	YNum [][FieldLimbs]uint64
	YDen [][FieldLimbs]uint64
}

// Map from the isogeny curve to the main curve using the parameters
func (p *IsogenyParams) Map(xIn, yIn *Field) (x, y *Field) {
	var xNum, xDen, yNum, yDen, tv [FieldLimbs]uint64
	var wasInverted int

	xnumL := len(p.XNum)
	xdenL := len(p.XDen)
	ynumL := len(p.YNum)
	ydenL := len(p.YDen)

	degree := 0
	for _, i := range []int{xnumL, xdenL, ynumL, ydenL} {
		if degree < i {
			degree = i
		}
	}

	xs := make([][FieldLimbs]uint64, degree)
	xs[0] = xIn.Params.R                      // x[0] = x^0
	xs[1] = xIn.Value                         // x[1] = x^1
	xIn.Arithmetic.Square(&xs[2], &xIn.Value) // x[2] = x^2
	for i := 3; i < degree; i++ {
		// x[i] = x^i
		xIn.Arithmetic.Mul(&xs[i], &xs[i-1], &xIn.Value)
	}

	computeIsoK(&xNum, &xs, &p.XNum, xIn.Arithmetic)
	computeIsoK(&xDen, &xs, &p.XDen, xIn.Arithmetic)
	computeIsoK(&yNum, &xs, &p.YNum, xIn.Arithmetic)
	computeIsoK(&yDen, &xs, &p.YDen, xIn.Arithmetic)

	xIn.Arithmetic.Invert(&wasInverted, &xDen, &xDen)
	x = new(Field).Set(xIn)
	xIn.Arithmetic.Mul(&tv, &xNum, &xDen)
	xIn.Arithmetic.Selectznz(&x.Value, &x.Value, &tv, wasInverted)

	yIn.Arithmetic.Invert(&wasInverted, &yDen, &yDen)
	y = new(Field).Set(yIn)
	yIn.Arithmetic.Mul(&tv, &yNum, &yDen)
	yIn.Arithmetic.Selectznz(&y.Value, &y.Value, &tv, wasInverted)
	yIn.Arithmetic.Mul(&y.Value, &y.Value, &yIn.Value)
	return
}

func computeIsoK(out *[FieldLimbs]uint64, xxs, k *[][FieldLimbs]uint64, f FieldArithmetic) {
	var tv [FieldLimbs]uint64

	for i := range *k {
		f.Mul(&tv, &(*xxs)[i], &(*k)[i])
		f.Add(out, out, &tv)
	}
}
