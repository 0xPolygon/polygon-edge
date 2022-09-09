package bls12381

import (
	"io"

	"github.com/coinbase/kryptology/internal"
	"github.com/coinbase/kryptology/pkg/core/curves/native"
)

// GtFieldBytes is the number of bytes needed to represent this field
const GtFieldBytes = 576

// Gt is the target group
type Gt fp12

// Random generates a random field element
func (gt *Gt) Random(reader io.Reader) (*Gt, error) {
	_, err := (*fp12)(gt).Random(reader)
	return gt, err
}

// FinalExponentiation performs a "final exponentiation" routine to convert the result
// of a Miller loop into an element of `Gt` with help of efficient squaring
// operation in the so-called `cyclotomic subgroup` of `Fq6` so that
// it can be compared with other elements of `Gt`.
func (gt *Gt) FinalExponentiation(a *Gt) *Gt {
	var t0, t1, t2, t3, t4, t5, t6, t fp12
	t0.FrobeniusMap((*fp12)(a))
	t0.FrobeniusMap(&t0)
	t0.FrobeniusMap(&t0)
	t0.FrobeniusMap(&t0)
	t0.FrobeniusMap(&t0)
	t0.FrobeniusMap(&t0)

	// Shouldn't happen since we enforce `a` to be non-zero but just in case
	_, wasInverted := t1.Invert((*fp12)(a))
	t2.Mul(&t0, &t1)
	t1.Set(&t2)
	t2.FrobeniusMap(&t2)
	t2.FrobeniusMap(&t2)
	t2.Mul(&t2, &t1)
	t1.cyclotomicSquare(&t2)
	t1.Conjugate(&t1)

	t3.cyclotomicExp(&t2)
	t4.cyclotomicSquare(&t3)
	t5.Mul(&t1, &t3)
	t1.cyclotomicExp(&t5)
	t0.cyclotomicExp(&t1)
	t6.cyclotomicExp(&t0)
	t6.Mul(&t6, &t4)
	t4.cyclotomicExp(&t6)
	t5.Conjugate(&t5)
	t4.Mul(&t4, &t5)
	t4.Mul(&t4, &t2)
	t5.Conjugate(&t2)
	t1.Mul(&t1, &t2)
	t1.FrobeniusMap(&t1)
	t1.FrobeniusMap(&t1)
	t1.FrobeniusMap(&t1)
	t6.Mul(&t6, &t5)
	t6.FrobeniusMap(&t6)
	t3.Mul(&t3, &t0)
	t3.FrobeniusMap(&t3)
	t3.FrobeniusMap(&t3)
	t3.Mul(&t3, &t1)
	t3.Mul(&t3, &t6)
	t.Mul(&t3, &t4)
	(*fp12)(gt).CMove((*fp12)(gt), &t, wasInverted)
	return gt
}

// IsZero returns 1 if gt == 0, 0 otherwise
func (gt *Gt) IsZero() int {
	return (*fp12)(gt).IsZero()
}

// IsOne returns 1 if gt == 1, 0 otherwise
func (gt *Gt) IsOne() int {
	return (*fp12)(gt).IsOne()
}

// SetOne gt = one
func (gt *Gt) SetOne() *Gt {
	(*fp12)(gt).SetOne()
	return gt
}

// Set copies a into gt
func (gt *Gt) Set(a *Gt) *Gt {
	gt.A.Set(&a.A)
	gt.B.Set(&a.B)
	return gt
}

// Bytes returns the Gt field byte representation
func (gt *Gt) Bytes() [GtFieldBytes]byte {
	var out [GtFieldBytes]byte
	t := gt.A.A.A.Bytes()
	copy(out[:FieldBytes], internal.ReverseScalarBytes(t[:]))
	t = gt.A.A.B.Bytes()
	copy(out[FieldBytes:2*FieldBytes], internal.ReverseScalarBytes(t[:]))
	t = gt.A.B.A.Bytes()
	copy(out[2*FieldBytes:3*FieldBytes], internal.ReverseScalarBytes(t[:]))
	t = gt.A.B.B.Bytes()
	copy(out[3*FieldBytes:4*FieldBytes], internal.ReverseScalarBytes(t[:]))
	t = gt.A.C.A.Bytes()
	copy(out[4*FieldBytes:5*FieldBytes], internal.ReverseScalarBytes(t[:]))
	t = gt.A.C.B.Bytes()
	copy(out[5*FieldBytes:6*FieldBytes], internal.ReverseScalarBytes(t[:]))
	t = gt.B.A.A.Bytes()
	copy(out[6*FieldBytes:7*FieldBytes], internal.ReverseScalarBytes(t[:]))
	t = gt.B.A.B.Bytes()
	copy(out[7*FieldBytes:8*FieldBytes], internal.ReverseScalarBytes(t[:]))
	t = gt.B.B.A.Bytes()
	copy(out[8*FieldBytes:9*FieldBytes], internal.ReverseScalarBytes(t[:]))
	t = gt.B.B.B.Bytes()
	copy(out[9*FieldBytes:10*FieldBytes], internal.ReverseScalarBytes(t[:]))
	t = gt.B.C.A.Bytes()
	copy(out[10*FieldBytes:11*FieldBytes], internal.ReverseScalarBytes(t[:]))
	t = gt.B.C.B.Bytes()
	copy(out[11*FieldBytes:12*FieldBytes], internal.ReverseScalarBytes(t[:]))

	return out
}

// SetBytes attempts to convert a big-endian byte representation of
// a scalar into a `Gt`, failing if the input is not canonical.
func (gt *Gt) SetBytes(input *[GtFieldBytes]byte) (*Gt, int) {
	var t [FieldBytes]byte
	var valid [12]int
	copy(t[:], internal.ReverseScalarBytes(input[:FieldBytes]))
	_, valid[0] = gt.A.A.A.SetBytes(&t)
	copy(t[:], internal.ReverseScalarBytes(input[FieldBytes:2*FieldBytes]))
	_, valid[1] = gt.A.A.B.SetBytes(&t)
	copy(t[:], internal.ReverseScalarBytes(input[2*FieldBytes:3*FieldBytes]))
	_, valid[2] = gt.A.B.A.SetBytes(&t)
	copy(t[:], internal.ReverseScalarBytes(input[3*FieldBytes:4*FieldBytes]))
	_, valid[3] = gt.A.B.B.SetBytes(&t)
	copy(t[:], internal.ReverseScalarBytes(input[4*FieldBytes:5*FieldBytes]))
	_, valid[4] = gt.A.C.A.SetBytes(&t)
	copy(t[:], internal.ReverseScalarBytes(input[5*FieldBytes:6*FieldBytes]))
	_, valid[5] = gt.A.C.B.SetBytes(&t)
	copy(t[:], internal.ReverseScalarBytes(input[6*FieldBytes:7*FieldBytes]))
	_, valid[6] = gt.B.A.A.SetBytes(&t)
	copy(t[:], internal.ReverseScalarBytes(input[7*FieldBytes:8*FieldBytes]))
	_, valid[7] = gt.B.A.B.SetBytes(&t)
	copy(t[:], internal.ReverseScalarBytes(input[8*FieldBytes:9*FieldBytes]))
	_, valid[8] = gt.B.B.A.SetBytes(&t)
	copy(t[:], internal.ReverseScalarBytes(input[9*FieldBytes:10*FieldBytes]))
	_, valid[9] = gt.B.B.B.SetBytes(&t)
	copy(t[:], internal.ReverseScalarBytes(input[10*FieldBytes:11*FieldBytes]))
	_, valid[10] = gt.B.C.A.SetBytes(&t)
	copy(t[:], internal.ReverseScalarBytes(input[11*FieldBytes:12*FieldBytes]))
	_, valid[11] = gt.B.C.B.SetBytes(&t)

	return gt, valid[0] & valid[1] &
		valid[2] & valid[3] &
		valid[4] & valid[5] &
		valid[6] & valid[7] &
		valid[8] & valid[9] &
		valid[10] & valid[11]
}

// Equal returns 1 if gt == rhs, 0 otherwise
func (gt *Gt) Equal(rhs *Gt) int {
	return (*fp12)(gt).Equal((*fp12)(rhs))
}

// Generator returns the base point
func (gt *Gt) Generator() *Gt {
	// pairing(&G1::generator(), &G2::generator())
	gt.Set((*Gt)(&fp12{
		A: fp6{
			A: fp2{
				A: fp{
					0x1972e433a01f85c5,
					0x97d32b76fd772538,
					0xc8ce546fc96bcdf9,
					0xcef63e7366d40614,
					0xa611342781843780,
					0x13f3448a3fc6d825,
				},
				B: fp{
					0xd26331b02e9d6995,
					0x9d68a482f7797e7d,
					0x9c9b29248d39ea92,
					0xf4801ca2e13107aa,
					0xa16c0732bdbcb066,
					0x083ca4afba360478,
				},
			},
			B: fp2{
				A: fp{
					0x59e261db0916b641,
					0x2716b6f4b23e960d,
					0xc8e55b10a0bd9c45,
					0x0bdb0bd99c4deda8,
					0x8cf89ebf57fdaac5,
					0x12d6b7929e777a5e,
				},
				B: fp{
					0x5fc85188b0e15f35,
					0x34a06e3a8f096365,
					0xdb3126a6e02ad62c,
					0xfc6f5aa97d9a990b,
					0xa12f55f5eb89c210,
					0x1723703a926f8889,
				},
			},
			C: fp2{
				A: fp{
					0x93588f2971828778,
					0x43f65b8611ab7585,
					0x3183aaf5ec279fdf,
					0xfa73d7e18ac99df6,
					0x64e176a6a64c99b0,
					0x179fa78c58388f1f,
				},
				B: fp{
					0x672a0a11ca2aef12,
					0x0d11b9b52aa3f16b,
					0xa44412d0699d056e,
					0xc01d0177221a5ba5,
					0x66e0cede6c735529,
					0x05f5a71e9fddc339,
				},
			},
		},
		B: fp6{
			A: fp2{
				A: fp{
					0xd30a88a1b062c679,
					0x5ac56a5d35fc8304,
					0xd0c834a6a81f290d,
					0xcd5430c2da3707c7,
					0xf0c27ff780500af0,
					0x09245da6e2d72eae,
				},
				B: fp{
					0x9f2e0676791b5156,
					0xe2d1c8234918fe13,
					0x4c9e459f3c561bf4,
					0xa3e85e53b9d3e3c1,
					0x820a121e21a70020,
					0x15af618341c59acc,
				},
			},
			B: fp2{
				A: fp{
					0x7c95658c24993ab1,
					0x73eb38721ca886b9,
					0x5256d749477434bc,
					0x8ba41902ea504a8b,
					0x04a3d3f80c86ce6d,
					0x18a64a87fb686eaa,
				},
				B: fp{
					0xbb83e71bb920cf26,
					0x2a5277ac92a73945,
					0xfc0ee59f94f046a0,
					0x7158cdf3786058f7,
					0x7cc1061b82f945f6,
					0x03f847aa9fdbe567,
				},
			},
			C: fp2{
				A: fp{
					0x8078dba56134e657,
					0x1cd7ec9a43998a6e,
					0xb1aa599a1a993766,
					0xc9a0f62f0842ee44,
					0x8e159be3b605dffa,
					0x0c86ba0d4af13fc2,
				},
				B: fp{
					0xe80ff2a06a52ffb1,
					0x7694ca48721a906c,
					0x7583183e03b08514,
					0xf567afdd40cee4e2,
					0x9a6d96d2e526a5fc,
					0x197e9f49861f2242,
				},
			},
		},
	}))
	return gt
}

// Add adds this value to another value.
func (gt *Gt) Add(arg1, arg2 *Gt) *Gt {
	(*fp12)(gt).Mul((*fp12)(arg1), (*fp12)(arg2))
	return gt
}

// Double this value
func (gt *Gt) Double(a *Gt) *Gt {
	(*fp12)(gt).Square((*fp12)(a))
	return gt
}

// Sub subtracts the two values
func (gt *Gt) Sub(arg1, arg2 *Gt) *Gt {
	var t fp12
	t.Conjugate((*fp12)(arg2))
	(*fp12)(gt).Mul((*fp12)(arg1), &t)
	return gt
}

// Neg negates this value
func (gt *Gt) Neg(a *Gt) *Gt {
	(*fp12)(gt).Conjugate((*fp12)(a))
	return gt
}

// Mul multiplies this value by the input scalar
func (gt *Gt) Mul(a *Gt, s *native.Field) *Gt {
	var f, p fp12
	f.Set((*fp12)(a))
	bytes := s.Bytes()

	precomputed := [16]fp12{}
	precomputed[1].Set(&f)
	for i := 2; i < 16; i += 2 {
		precomputed[i].Square(&precomputed[i>>1])
		precomputed[i+1].Mul(&precomputed[i], &f)
	}
	for i := 0; i < 256; i += 4 {
		// Brouwer / windowing method. window size of 4.
		for j := 0; j < 4; j++ {
			p.Square(&p)
		}
		window := bytes[32-1-i>>3] >> (4 - i&0x04) & 0x0F
		p.Mul(&p, &precomputed[window])
	}
	(*fp12)(gt).Set(&p)
	return gt
}

// Square this value
func (gt *Gt) Square(a *Gt) *Gt {
	(*fp12)(gt).cyclotomicSquare((*fp12)(a))
	return gt
}

// Invert this value
func (gt *Gt) Invert(a *Gt) (*Gt, int) {
	_, wasInverted := (*fp12)(gt).Invert((*fp12)(a))
	return gt, wasInverted
}

func fp4Square(a, b, arg1, arg2 *fp2) {
	var t0, t1, t2 fp2

	t0.Square(arg1)
	t1.Square(arg2)
	t2.MulByNonResidue(&t1)
	a.Add(&t2, &t0)
	t2.Add(arg1, arg2)
	t2.Square(&t2)
	t2.Sub(&t2, &t0)
	b.Sub(&t2, &t1)
}

func (f *fp12) cyclotomicSquare(a *fp12) *fp12 {
	// Adaptation of Algorithm 5.5.4, Guide to Pairing-Based Cryptography
	// Faster Squaring in the Cyclotomic Subgroup of Sixth Degree Extensions
	// https://eprint.iacr.org/2009/565.pdf
	var z0, z1, z2, z3, z4, z5, t0, t1, t2, t3 fp2
	z0.Set(&a.A.A)
	z4.Set(&a.A.B)
	z3.Set(&a.A.C)
	z2.Set(&a.B.A)
	z1.Set(&a.B.B)
	z5.Set(&a.B.C)

	fp4Square(&t0, &t1, &z0, &z1)
	z0.Sub(&t0, &z0)
	z0.Double(&z0)
	z0.Add(&z0, &t0)

	z1.Add(&t1, &z1)
	z1.Double(&z1)
	z1.Add(&z1, &t1)

	fp4Square(&t0, &t1, &z2, &z3)
	fp4Square(&t2, &t3, &z4, &z5)

	z4.Sub(&t0, &z4)
	z4.Double(&z4)
	z4.Add(&z4, &t0)

	z5.Add(&z5, &t1)
	z5.Double(&z5)
	z5.Add(&z5, &t1)

	t0.MulByNonResidue(&t3)
	z2.Add(&z2, &t0)
	z2.Double(&z2)
	z2.Add(&z2, &t0)

	z3.Sub(&t2, &z3)
	z3.Double(&z3)
	z3.Add(&z3, &t2)

	f.A.A.Set(&z0)
	f.A.B.Set(&z4)
	f.A.C.Set(&z3)

	f.B.A.Set(&z2)
	f.B.B.Set(&z1)
	f.B.C.Set(&z5)
	return f
}

func (f *fp12) cyclotomicExp(a *fp12) *fp12 {
	var t fp12
	t.SetOne()
	foundOne := 0

	for i := 63; i >= 0; i-- {
		b := int((paramX >> i) & 1)
		if foundOne == 1 {
			t.cyclotomicSquare(&t)
		} else {
			foundOne = b
		}
		if b == 1 {
			t.Mul(&t, a)
		}
	}
	f.Conjugate(&t)
	return f
}
