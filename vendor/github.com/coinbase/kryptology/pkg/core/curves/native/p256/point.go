package p256

import (
	"sync"

	"github.com/coinbase/kryptology/internal"
	"github.com/coinbase/kryptology/pkg/core/curves/native"
	"github.com/coinbase/kryptology/pkg/core/curves/native/p256/fp"
)

var (
	p256PointInitonce     sync.Once
	p256PointParams       native.EllipticPointParams
	p256PointSswuInitOnce sync.Once
	p256PointSswuParams   native.SswuParams
)

func P256PointNew() *native.EllipticPoint {
	return &native.EllipticPoint{
		X:          fp.P256FpNew(),
		Y:          fp.P256FpNew(),
		Z:          fp.P256FpNew(),
		Params:     getP256PointParams(),
		Arithmetic: &p256PointArithmetic{},
	}
}

func p256PointParamsInit() {
	// How these values were derived
	// left for informational purposes
	//params := elliptic.P256().Params()
	//a := big.NewInt(-3)
	//a.Mod(a, params.P)
	//capA := fp.P256FpNew().SetBigInt(a)
	//capB := fp.P256FpNew().SetBigInt(params.B)
	//gx := fp.P256FpNew().SetBigInt(params.Gx)
	//gy := fp.P256FpNew().SetBigInt(params.Gy)

	p256PointParams = native.EllipticPointParams{
		A:       fp.P256FpNew().SetRaw(&[native.FieldLimbs]uint64{0xfffffffffffffffc, 0x00000003ffffffff, 0x0000000000000000, 0xfffffffc00000004}),
		B:       fp.P256FpNew().SetRaw(&[native.FieldLimbs]uint64{0xd89cdf6229c4bddf, 0xacf005cd78843090, 0xe5a220abf7212ed6, 0xdc30061d04874834}),
		Gx:      fp.P256FpNew().SetRaw(&[native.FieldLimbs]uint64{0x79e730d418a9143c, 0x75ba95fc5fedb601, 0x79fb732b77622510, 0x18905f76a53755c6}),
		Gy:      fp.P256FpNew().SetRaw(&[native.FieldLimbs]uint64{0xddf25357ce95560a, 0x8b4ab8e4ba19e45c, 0xd2e88688dd21f325, 0x8571ff1825885d85}),
		BitSize: 256,
		Name:    "P256",
	}
}

func getP256PointParams() *native.EllipticPointParams {
	p256PointInitonce.Do(p256PointParamsInit)
	return &p256PointParams
}

func getP256PointSswuParams() *native.SswuParams {
	p256PointSswuInitOnce.Do(p256PointSswuParamsInit)
	return &p256PointSswuParams
}

func p256PointSswuParamsInit() {
	// How these values were derived
	// left for informational purposes
	//params := elliptic.P256().Params()
	//
	//// c1 = (q - 3) / 4
	//c1 := new(big.Int).Set(params.P)
	//c1.Sub(c1, big.NewInt(3))
	//c1.Rsh(c1, 2)
	//
	//a := big.NewInt(-3)
	//a.Mod(a, params.P)
	//b := new(big.Int).Set(params.B)
	//z := big.NewInt(-10)
	//z.Mod(z, params.P)
	//// sqrt(-Z^3)
	//zTmp := new(big.Int).Exp(z, big.NewInt(3), nil)
	//zTmp = zTmp.Neg(zTmp)
	//zTmp.Mod(zTmp, params.P)
	//c2 := new(big.Int).ModSqrt(zTmp, params.P)
	//
	//var capC1Bytes [32]byte
	//c1.FillBytes(capC1Bytes[:])
	//capC1 := fp.P256FpNew().SetRaw(&[native.FieldLimbs]uint64{
	//	binary.BigEndian.Uint64(capC1Bytes[24:]),
	//	binary.BigEndian.Uint64(capC1Bytes[16:24]),
	//	binary.BigEndian.Uint64(capC1Bytes[8:16]),
	//	binary.BigEndian.Uint64(capC1Bytes[:8]),
	//})
	//capC2 := fp.P256FpNew().SetBigInt(c2)
	//capA := fp.P256FpNew().SetBigInt(a)
	//capB := fp.P256FpNew().SetBigInt(b)
	//capZ := fp.P256FpNew().SetBigInt(z)

	p256PointSswuParams = native.SswuParams{
		C1: [native.FieldLimbs]uint64{0xffffffffffffffff, 0x000000003fffffff, 0x4000000000000000, 0x3fffffffc0000000},
		C2: [native.FieldLimbs]uint64{0x53e43951f64fdbe7, 0xb2806c63966a1a66, 0x1ac5d59c3298bf50, 0xa3323851ba997e27},
		A:  [native.FieldLimbs]uint64{0xfffffffffffffffc, 0x00000003ffffffff, 0x0000000000000000, 0xfffffffc00000004},
		B:  [native.FieldLimbs]uint64{0xd89cdf6229c4bddf, 0xacf005cd78843090, 0xe5a220abf7212ed6, 0xdc30061d04874834},
		Z:  [native.FieldLimbs]uint64{0xfffffffffffffff5, 0x0000000affffffff, 0x0000000000000000, 0xfffffff50000000b},
	}
}

type p256PointArithmetic struct{}

func (k p256PointArithmetic) Hash(out *native.EllipticPoint, hash *native.EllipticPointHasher, msg, dst []byte) error {
	var u []byte
	sswuParams := getP256PointSswuParams()

	switch hash.Type() {
	case native.XMD:
		u = native.ExpandMsgXmd(hash, msg, dst, 96)
	case native.XOF:
		u = native.ExpandMsgXof(hash, msg, dst, 96)
	}
	var buf [64]byte
	copy(buf[:48], internal.ReverseScalarBytes(u[:48]))
	u0 := fp.P256FpNew().SetBytesWide(&buf)
	copy(buf[:48], internal.ReverseScalarBytes(u[48:]))
	u1 := fp.P256FpNew().SetBytesWide(&buf)

	q0x, q0y := sswuParams.Osswu3mod4(u0)
	q1x, q1y := sswuParams.Osswu3mod4(u1)
	out.X = q0x
	out.Y = q0y
	out.Z.SetOne()
	tv := &native.EllipticPoint{
		X: q1x,
		Y: q1y,
		Z: fp.P256FpNew().SetOne(),
	}
	k.Add(out, out, tv)
	return nil
}

func (k p256PointArithmetic) Double(out, arg *native.EllipticPoint) {
	// Addition formula from Renes-Costello-Batina 2015
	// (https://eprint.iacr.org/2015/1060 Algorithm 6)
	var xx, yy, zz, xy2, yz2, xz2, bzz, bzz3 [native.FieldLimbs]uint64
	var yyMBzz3, yyPBzz3, yFrag, xFrag, zz3 [native.FieldLimbs]uint64
	var bxz2, bxz6, xx3Mzz3, x, y, z [native.FieldLimbs]uint64
	b := getP256PointParams().B.Value
	f := arg.X.Arithmetic

	f.Square(&xx, &arg.X.Value)
	f.Square(&yy, &arg.Y.Value)
	f.Square(&zz, &arg.Z.Value)

	f.Mul(&xy2, &arg.X.Value, &arg.Y.Value)
	f.Add(&xy2, &xy2, &xy2)

	f.Mul(&yz2, &arg.Y.Value, &arg.Z.Value)
	f.Add(&yz2, &yz2, &yz2)

	f.Mul(&xz2, &arg.X.Value, &arg.Z.Value)
	f.Add(&xz2, &xz2, &xz2)

	f.Mul(&bzz, &b, &zz)
	f.Sub(&bzz, &bzz, &xz2)

	f.Add(&bzz3, &bzz, &bzz)
	f.Add(&bzz3, &bzz3, &bzz)

	f.Sub(&yyMBzz3, &yy, &bzz3)
	f.Add(&yyPBzz3, &yy, &bzz3)
	f.Mul(&yFrag, &yyPBzz3, &yyMBzz3)
	f.Mul(&xFrag, &yyMBzz3, &xy2)

	f.Add(&zz3, &zz, &zz)
	f.Add(&zz3, &zz3, &zz)

	f.Mul(&bxz2, &b, &xz2)
	f.Sub(&bxz2, &bxz2, &zz3)
	f.Sub(&bxz2, &bxz2, &xx)

	f.Add(&bxz6, &bxz2, &bxz2)
	f.Add(&bxz6, &bxz6, &bxz2)

	f.Add(&xx3Mzz3, &xx, &xx)
	f.Add(&xx3Mzz3, &xx3Mzz3, &xx)
	f.Sub(&xx3Mzz3, &xx3Mzz3, &zz3)

	f.Mul(&x, &bxz6, &yz2)
	f.Sub(&x, &xFrag, &x)

	f.Mul(&y, &xx3Mzz3, &bxz6)
	f.Add(&y, &yFrag, &y)

	f.Mul(&z, &yz2, &yy)
	f.Add(&z, &z, &z)
	f.Add(&z, &z, &z)

	out.X.Value = x
	out.Y.Value = y
	out.Z.Value = z
}

func (k p256PointArithmetic) Add(out, arg1, arg2 *native.EllipticPoint) {
	// Addition formula from Renes-Costello-Batina 2015
	// (https://eprint.iacr.org/2015/1060 Algorithm 4).
	var xx, yy, zz, zz3, bxz, bxz3 [native.FieldLimbs]uint64
	var tv1, xyPairs, yzPairs, xzPairs [native.FieldLimbs]uint64
	var bzz, bzz3, yyMBzz3, yyPBzz3 [native.FieldLimbs]uint64
	var xx3Mzz3, x, y, z [native.FieldLimbs]uint64
	f := arg1.X.Arithmetic
	b := getP256PointParams().B.Value

	f.Mul(&xx, &arg1.X.Value, &arg2.X.Value)
	f.Mul(&yy, &arg1.Y.Value, &arg2.Y.Value)
	f.Mul(&zz, &arg1.Z.Value, &arg2.Z.Value)

	f.Add(&tv1, &arg2.X.Value, &arg2.Y.Value)
	f.Add(&xyPairs, &arg1.X.Value, &arg1.Y.Value)
	f.Mul(&xyPairs, &xyPairs, &tv1)
	f.Sub(&xyPairs, &xyPairs, &xx)
	f.Sub(&xyPairs, &xyPairs, &yy)

	f.Add(&tv1, &arg2.Y.Value, &arg2.Z.Value)
	f.Add(&yzPairs, &arg1.Y.Value, &arg1.Z.Value)
	f.Mul(&yzPairs, &yzPairs, &tv1)
	f.Sub(&yzPairs, &yzPairs, &yy)
	f.Sub(&yzPairs, &yzPairs, &zz)

	f.Add(&tv1, &arg2.X.Value, &arg2.Z.Value)
	f.Add(&xzPairs, &arg1.X.Value, &arg1.Z.Value)
	f.Mul(&xzPairs, &xzPairs, &tv1)
	f.Sub(&xzPairs, &xzPairs, &xx)
	f.Sub(&xzPairs, &xzPairs, &zz)

	f.Mul(&bzz, &b, &zz)
	f.Sub(&bzz, &xzPairs, &bzz)

	f.Add(&bzz3, &bzz, &bzz)
	f.Add(&bzz3, &bzz3, &bzz)

	f.Sub(&yyMBzz3, &yy, &bzz3)
	f.Add(&yyPBzz3, &yy, &bzz3)

	f.Add(&zz3, &zz, &zz)
	f.Add(&zz3, &zz3, &zz)

	f.Mul(&bxz, &b, &xzPairs)
	f.Sub(&bxz, &bxz, &zz3)
	f.Sub(&bxz, &bxz, &xx)

	f.Add(&bxz3, &bxz, &bxz)
	f.Add(&bxz3, &bxz3, &bxz)

	f.Add(&xx3Mzz3, &xx, &xx)
	f.Add(&xx3Mzz3, &xx3Mzz3, &xx)
	f.Sub(&xx3Mzz3, &xx3Mzz3, &zz3)

	f.Mul(&tv1, &yzPairs, &bxz3)
	f.Mul(&x, &yyPBzz3, &xyPairs)
	f.Sub(&x, &x, &tv1)

	f.Mul(&tv1, &xx3Mzz3, &bxz3)
	f.Mul(&y, &yyPBzz3, &yyMBzz3)
	f.Add(&y, &y, &tv1)

	f.Mul(&tv1, &xyPairs, &xx3Mzz3)
	f.Mul(&z, &yyMBzz3, &yzPairs)
	f.Add(&z, &z, &tv1)

	e1 := arg1.Z.IsZero()
	e2 := arg2.Z.IsZero()

	// If arg1 is identity set it to arg2
	f.Selectznz(&z, &z, &arg2.Z.Value, e1)
	f.Selectznz(&y, &y, &arg2.Y.Value, e1)
	f.Selectznz(&x, &x, &arg2.X.Value, e1)
	// If arg2 is identity set it to arg1
	f.Selectznz(&z, &z, &arg1.Z.Value, e2)
	f.Selectznz(&y, &y, &arg1.Y.Value, e2)
	f.Selectznz(&x, &x, &arg1.X.Value, e2)

	out.X.Value = x
	out.Y.Value = y
	out.Z.Value = z
}

func (k p256PointArithmetic) IsOnCurve(arg *native.EllipticPoint) bool {
	affine := P256PointNew()
	k.ToAffine(affine, arg)
	lhs := fp.P256FpNew().Square(affine.Y)
	rhs := fp.P256FpNew()
	k.RhsEq(rhs, affine.X)
	return lhs.Equal(rhs) == 1
}

func (k p256PointArithmetic) ToAffine(out, arg *native.EllipticPoint) {
	var wasInverted int
	var zero, x, y, z [native.FieldLimbs]uint64
	f := arg.X.Arithmetic

	f.Invert(&wasInverted, &z, &arg.Z.Value)
	f.Mul(&x, &arg.X.Value, &z)
	f.Mul(&y, &arg.Y.Value, &z)

	out.Z.SetOne()
	// If point at infinity this does nothing
	f.Selectznz(&x, &zero, &x, wasInverted)
	f.Selectznz(&y, &zero, &y, wasInverted)
	f.Selectznz(&z, &zero, &out.Z.Value, wasInverted)

	out.X.Value = x
	out.Y.Value = y
	out.Z.Value = z
	out.Params = arg.Params
	out.Arithmetic = arg.Arithmetic
}

func (k p256PointArithmetic) RhsEq(out, x *native.Field) {
	// Elliptic curve equation for p256 is: y^2 = x^3 ax + b
	out.Square(x)
	out.Mul(out, x)
	out.Add(out, getP256PointParams().B)
	out.Add(out, fp.P256FpNew().Mul(getP256PointParams().A, x))
}
