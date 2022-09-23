package k256

import (
	"sync"

	"github.com/coinbase/kryptology/internal"
	"github.com/coinbase/kryptology/pkg/core/curves/native"
	"github.com/coinbase/kryptology/pkg/core/curves/native/k256/fp"
)

var (
	k256PointInitonce        sync.Once
	k256PointParams          native.EllipticPointParams
	k256PointSswuInitOnce    sync.Once
	k256PointSswuParams      native.SswuParams
	k256PointIsogenyInitOnce sync.Once
	k256PointIsogenyParams   native.IsogenyParams
)

func K256PointNew() *native.EllipticPoint {
	return &native.EllipticPoint{
		X:          fp.K256FpNew(),
		Y:          fp.K256FpNew(),
		Z:          fp.K256FpNew(),
		Params:     getK256PointParams(),
		Arithmetic: &k256PointArithmetic{},
	}
}

func k256PointParamsInit() {
	k256PointParams = native.EllipticPointParams{
		A: fp.K256FpNew(),
		B: fp.K256FpNew().SetUint64(7),
		Gx: fp.K256FpNew().SetLimbs(&[native.FieldLimbs]uint64{
			0x59f2815b16f81798,
			0x029bfcdb2dce28d9,
			0x55a06295ce870b07,
			0x79be667ef9dcbbac,
		}),
		Gy: fp.K256FpNew().SetLimbs(&[native.FieldLimbs]uint64{
			0x9c47d08ffb10d4b8,
			0xfd17b448a6855419,
			0x5da4fbfc0e1108a8,
			0x483ada7726a3c465,
		}),
		BitSize: 256,
		Name:    "secp256k1",
	}
}

func getK256PointParams() *native.EllipticPointParams {
	k256PointInitonce.Do(k256PointParamsInit)
	return &k256PointParams
}

func getK256PointSswuParams() *native.SswuParams {
	k256PointSswuInitOnce.Do(k256PointSswuParamsInit)
	return &k256PointSswuParams
}

func k256PointSswuParamsInit() {
	// Taken from https://datatracker.ietf.org/doc/html/draft-irtf-cfrg-hash-to-curve-11#section-8.7
	//params := btcec.S256().Params()
	//
	//// c1 = (q - 3) / 4
	//c1 := new(big.Int).Set(params.P)
	//c1.Sub(c1, big.NewInt(3))
	//c1.Rsh(c1, 2)
	//
	//a, _ := new(big.Int).SetString("3f8731abdd661adca08a5558f0f5d272e953d363cb6f0e5d405447c01a444533", 16)
	//b := big.NewInt(1771)
	//z := big.NewInt(-11)
	//z.Mod(z, params.P)
	//// sqrt(-z^3)
	//zTmp := new(big.Int).Exp(z, big.NewInt(3), nil)
	//zTmp = zTmp.Neg(zTmp)
	//zTmp.Mod(zTmp, params.P)
	//c2 := new(big.Int).ModSqrt(zTmp, params.P)
	//
	//var tBytes [32]byte
	//c1.FillBytes(tBytes[:])
	//newC1 := [native.FieldLimbs]uint64{
	//	binary.BigEndian.Uint64(tBytes[24:32]),
	//	binary.BigEndian.Uint64(tBytes[16:24]),
	//	binary.BigEndian.Uint64(tBytes[8:16]),
	//	binary.BigEndian.Uint64(tBytes[:8]),
	//}
	//fp.K256FpNew().Arithmetic.ToMontgomery(&newC1, &newC1)
	//c2.FillBytes(tBytes[:])
	//newC2 := [native.FieldLimbs]uint64{
	//	binary.BigEndian.Uint64(tBytes[24:32]),
	//	binary.BigEndian.Uint64(tBytes[16:24]),
	//	binary.BigEndian.Uint64(tBytes[8:16]),
	//	binary.BigEndian.Uint64(tBytes[:8]),
	//}
	//fp.K256FpNew().Arithmetic.ToMontgomery(&newC2, &newC2)
	//a.FillBytes(tBytes[:])
	//newA := [native.FieldLimbs]uint64{
	//	binary.BigEndian.Uint64(tBytes[24:32]),
	//	binary.BigEndian.Uint64(tBytes[16:24]),
	//	binary.BigEndian.Uint64(tBytes[8:16]),
	//	binary.BigEndian.Uint64(tBytes[:8]),
	//}
	//fp.K256FpNew().Arithmetic.ToMontgomery(&newA, &newA)
	//b.FillBytes(tBytes[:])
	//newB := [native.FieldLimbs]uint64{
	//	binary.BigEndian.Uint64(tBytes[24:32]),
	//	binary.BigEndian.Uint64(tBytes[16:24]),
	//	binary.BigEndian.Uint64(tBytes[8:16]),
	//	binary.BigEndian.Uint64(tBytes[:8]),
	//}
	//fp.K256FpNew().Arithmetic.ToMontgomery(&newB, &newB)
	//z.FillBytes(tBytes[:])
	//newZ := [native.FieldLimbs]uint64{
	//	binary.BigEndian.Uint64(tBytes[24:32]),
	//	binary.BigEndian.Uint64(tBytes[16:24]),
	//	binary.BigEndian.Uint64(tBytes[8:16]),
	//	binary.BigEndian.Uint64(tBytes[:8]),
	//}
	//fp.K256FpNew().Arithmetic.ToMontgomery(&newZ, &newZ)

	k256PointSswuParams = native.SswuParams{
		// (q -3) // 4
		C1: [native.FieldLimbs]uint64{0xffffffffbfffff0b, 0xffffffffffffffff, 0xffffffffffffffff, 0x3fffffffffffffff},
		// sqrt(-z^3)
		C2: [native.FieldLimbs]uint64{0x5b57ba53a30d1520, 0x908f7cef34a762eb, 0x190b0ffe068460c8, 0x98a9828e8f00ff62},
		// 0x3f8731abdd661adca08a5558f0f5d272e953d363cb6f0e5d405447c01a444533
		A: [native.FieldLimbs]uint64{0xdb714ce7b18444a1, 0x4458ce38a32a19a2, 0xa0e58ae2837bfbf0, 0x505aabc49336d959},
		// 1771
		B: [native.FieldLimbs]uint64{0x000006eb001a66db, 0x0000000000000000, 0x0000000000000000, 0x0000000000000000},
		// -11
		Z: [native.FieldLimbs]uint64{0xfffffff3ffffd234, 0xffffffffffffffff, 0xffffffffffffffff, 0xffffffffffffffff},
	}
}

func k256PointIsogenyInit() {
	k256PointIsogenyParams = native.IsogenyParams{
		XNum: [][native.FieldLimbs]uint64{
			{
				0x0000003b1c72a8b4,
				0x0000000000000000,
				0x0000000000000000,
				0x0000000000000000,
			},
			{
				0xd5bd51a17b2edf46,
				0x2cc06f7c86b86bcd,
				0x50b37e74f3294a00,
				0xeb32314a9da73679,
			},
			{
				0x48c18b1b0d2191bd,
				0x5a3f74c29bfccce3,
				0xbe55a02e5e8bd357,
				0x09bf218d11fff905,
			},
			{
				0x000000001c71c789,
				0x0000000000000000,
				0x0000000000000000,
				0x0000000000000000,
			},
		},
		XDen: [][native.FieldLimbs]uint64{
			{
				0x8af79c1ffdf1e7fa,
				0xb84bc22235735eb5,
				0x82ee5655a55ace04,
				0xce4b32dea0a2becb,
			},
			{
				0x8ecde3f3762e1fa5,
				0x2c3b1ad77be333fd,
				0xb102a1a152ea6e12,
				0x57b82df5a1ffc133,
			},
			{
				0x00000001000003d1,
				0x0000000000000000,
				0x0000000000000000,
				0x0000000000000000,
			},
		},
		YNum: [][native.FieldLimbs]uint64{
			{
				0xffffffce425e12c3,
				0xffffffffffffffff,
				0xffffffffffffffff,
				0xffffffffffffffff,
			},
			{
				0xba60d5fd6e56922e,
				0x4ec198c898a435f2,
				0x27e77a577b9764ab,
				0xb3b80a1197651d12,
			},
			{
				0xa460c58d0690c6f6,
				0xad1fba614dfe6671,
				0xdf2ad0172f45e9ab,
				0x84df90c688fffc82,
			},
			{
				0x00000000097b4283,
				0x0000000000000000,
				0x0000000000000000,
				0x0000000000000000,
			},
		},
		YDen: [][native.FieldLimbs]uint64{
			{
				0xfffffd0afff4b6fb,
				0xffffffffffffffff,
				0xffffffffffffffff,
				0xffffffffffffffff,
			},
			{
				0xa0e6d461f9d5bf90,
				0x28e34666a05a1c20,
				0x88cb0300f0106a0e,
				0x6ae1989be1e83c62,
			},
			{
				0x5634d5edb1453160,
				0x4258a84339d4cdfc,
				0x8983f271fc5fa51b,
				0x039444f072ffa1cd,
			},
			{
				0x00000001000003d1,
				0x0000000000000000,
				0x0000000000000000,
				0x0000000000000000,
			},
		},
	}
}

func getK256PointIsogenyParams() *native.IsogenyParams {
	k256PointIsogenyInitOnce.Do(k256PointIsogenyInit)
	return &k256PointIsogenyParams
}

type k256PointArithmetic struct{}

func (k k256PointArithmetic) Hash(out *native.EllipticPoint, hash *native.EllipticPointHasher, msg, dst []byte) error {
	var u []byte
	sswuParams := getK256PointSswuParams()
	isoParams := getK256PointIsogenyParams()

	switch hash.Type() {
	case native.XMD:
		u = native.ExpandMsgXmd(hash, msg, dst, 96)
	case native.XOF:
		u = native.ExpandMsgXof(hash, msg, dst, 96)
	}
	var buf [64]byte
	copy(buf[:48], internal.ReverseScalarBytes(u[:48]))
	u0 := fp.K256FpNew().SetBytesWide(&buf)
	copy(buf[:48], internal.ReverseScalarBytes(u[48:]))
	u1 := fp.K256FpNew().SetBytesWide(&buf)

	r0x, r0y := sswuParams.Osswu3mod4(u0)
	r1x, r1y := sswuParams.Osswu3mod4(u1)
	q0x, q0y := isoParams.Map(r0x, r0y)
	q1x, q1y := isoParams.Map(r1x, r1y)
	out.X = q0x
	out.Y = q0y
	out.Z.SetOne()
	tv := &native.EllipticPoint{
		X: q1x,
		Y: q1y,
		Z: fp.K256FpNew().SetOne(),
	}
	k.Add(out, out, tv)
	return nil
}

func (k k256PointArithmetic) Double(out, arg *native.EllipticPoint) {
	// Addition formula from Renes-Costello-Batina 2015
	// (https://eprint.iacr.org/2015/1060 Algorithm 9)
	var yy, zz, xy2, bzz, bzz3, bzz9 [native.FieldLimbs]uint64
	var yyMBzz9, yyPBzz3, yyzz, yyzz8, t [native.FieldLimbs]uint64
	var x, y, z [native.FieldLimbs]uint64
	f := arg.X.Arithmetic

	f.Square(&yy, &arg.Y.Value)
	f.Square(&zz, &arg.Z.Value)
	f.Mul(&xy2, &arg.X.Value, &arg.Y.Value)
	f.Add(&xy2, &xy2, &xy2)
	f.Mul(&bzz, &zz, &arg.Params.B.Value)
	f.Add(&bzz3, &bzz, &bzz)
	f.Add(&bzz3, &bzz3, &bzz)
	f.Add(&bzz9, &bzz3, &bzz3)
	f.Add(&bzz9, &bzz9, &bzz3)
	f.Neg(&yyMBzz9, &bzz9)
	f.Add(&yyMBzz9, &yyMBzz9, &yy)
	f.Add(&yyPBzz3, &yy, &bzz3)
	f.Mul(&yyzz, &yy, &zz)
	f.Add(&yyzz8, &yyzz, &yyzz)
	f.Add(&yyzz8, &yyzz8, &yyzz8)
	f.Add(&yyzz8, &yyzz8, &yyzz8)
	f.Add(&t, &yyzz8, &yyzz8)
	f.Add(&t, &t, &yyzz8)
	f.Mul(&t, &t, &arg.Params.B.Value)

	f.Mul(&x, &xy2, &yyMBzz9)

	f.Mul(&y, &yyMBzz9, &yyPBzz3)
	f.Add(&y, &y, &t)

	f.Mul(&z, &yy, &arg.Y.Value)
	f.Mul(&z, &z, &arg.Z.Value)
	f.Add(&z, &z, &z)
	f.Add(&z, &z, &z)
	f.Add(&z, &z, &z)

	out.X.Value = x
	out.Y.Value = y
	out.Z.Value = z
}

func (k k256PointArithmetic) Add(out, arg1, arg2 *native.EllipticPoint) {
	// Addition formula from Renes-Costello-Batina 2015
	// (https://eprint.iacr.org/2015/1060 Algorithm 7).
	var xx, yy, zz, nXxYy, nYyZz, nXxZz [native.FieldLimbs]uint64
	var tv1, tv2, xyPairs, yzPairs, xzPairs [native.FieldLimbs]uint64
	var bzz, bzz3, yyMBzz3, yyPBzz3, byz [native.FieldLimbs]uint64
	var byz3, xx3, bxx9, x, y, z [native.FieldLimbs]uint64
	f := arg1.X.Arithmetic

	f.Mul(&xx, &arg1.X.Value, &arg2.X.Value)
	f.Mul(&yy, &arg1.Y.Value, &arg2.Y.Value)
	f.Mul(&zz, &arg1.Z.Value, &arg2.Z.Value)

	f.Add(&nXxYy, &xx, &yy)
	f.Neg(&nXxYy, &nXxYy)

	f.Add(&nYyZz, &yy, &zz)
	f.Neg(&nYyZz, &nYyZz)

	f.Add(&nXxZz, &xx, &zz)
	f.Neg(&nXxZz, &nXxZz)

	f.Add(&tv1, &arg1.X.Value, &arg1.Y.Value)
	f.Add(&tv2, &arg2.X.Value, &arg2.Y.Value)
	f.Mul(&xyPairs, &tv1, &tv2)
	f.Add(&xyPairs, &xyPairs, &nXxYy)

	f.Add(&tv1, &arg1.Y.Value, &arg1.Z.Value)
	f.Add(&tv2, &arg2.Y.Value, &arg2.Z.Value)
	f.Mul(&yzPairs, &tv1, &tv2)
	f.Add(&yzPairs, &yzPairs, &nYyZz)

	f.Add(&tv1, &arg1.X.Value, &arg1.Z.Value)
	f.Add(&tv2, &arg2.X.Value, &arg2.Z.Value)
	f.Mul(&xzPairs, &tv1, &tv2)
	f.Add(&xzPairs, &xzPairs, &nXxZz)

	f.Mul(&bzz, &zz, &arg1.Params.B.Value)
	f.Add(&bzz3, &bzz, &bzz)
	f.Add(&bzz3, &bzz3, &bzz)

	f.Neg(&yyMBzz3, &bzz3)
	f.Add(&yyMBzz3, &yyMBzz3, &yy)

	f.Add(&yyPBzz3, &yy, &bzz3)

	f.Mul(&byz, &yzPairs, &arg1.Params.B.Value)
	f.Add(&byz3, &byz, &byz)
	f.Add(&byz3, &byz3, &byz)

	f.Add(&xx3, &xx, &xx)
	f.Add(&xx3, &xx3, &xx)

	f.Add(&bxx9, &xx3, &xx3)
	f.Add(&bxx9, &bxx9, &xx3)
	f.Mul(&bxx9, &bxx9, &arg1.Params.B.Value)

	f.Mul(&tv1, &xyPairs, &yyMBzz3)
	f.Mul(&tv2, &byz3, &xzPairs)
	f.Neg(&tv2, &tv2)
	f.Add(&x, &tv1, &tv2)

	f.Mul(&tv1, &yyPBzz3, &yyMBzz3)
	f.Mul(&tv2, &bxx9, &xzPairs)
	f.Add(&y, &tv1, &tv2)

	f.Mul(&tv1, &yzPairs, &yyPBzz3)
	f.Mul(&tv2, &xx3, &xyPairs)
	f.Add(&z, &tv1, &tv2)

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

func (k k256PointArithmetic) IsOnCurve(arg *native.EllipticPoint) bool {
	affine := K256PointNew()
	k.ToAffine(affine, arg)
	lhs := fp.K256FpNew().Square(affine.Y)
	rhs := fp.K256FpNew()
	k.RhsEq(rhs, affine.X)
	return lhs.Equal(rhs) == 1
}

func (k k256PointArithmetic) ToAffine(out, arg *native.EllipticPoint) {
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

func (k k256PointArithmetic) RhsEq(out, x *native.Field) {
	// Elliptic curve equation for secp256k1 is: y^2 = x^3 + 7
	out.Square(x)
	out.Mul(out, x)
	out.Add(out, getK256PointParams().B)
}
