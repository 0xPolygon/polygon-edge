package bls12381

import (
	"fmt"
	"io"
	"math/big"

	"github.com/pkg/errors"

	"github.com/coinbase/kryptology/internal"
	"github.com/coinbase/kryptology/pkg/core/curves/native"
)

var (
	g1x = fp{
		0x5cb38790fd530c16,
		0x7817fc679976fff5,
		0x154f95c7143ba1c1,
		0xf0ae6acdf3d0e747,
		0xedce6ecc21dbf440,
		0x120177419e0bfb75,
	}
	g1y = fp{
		0xbaac93d50ce72271,
		0x8c22631a7918fd8e,
		0xdd595f13570725ce,
		0x51ac582950405194,
		0x0e1c8c3fad0059c0,
		0x0bbc3efc5008a26a,
	}
	curveG1B = fp{
		0xaa270000000cfff3,
		0x53cc0032fc34000a,
		0x478fe97a6b0a807f,
		0xb1d37ebee6ba24d7,
		0x8ec9733bbf78ab2f,
		0x09d645513d83de7e,
	}
	osswuMapA = fp{
		0x2f65aa0e9af5aa51,
		0x86464c2d1e8416c3,
		0xb85ce591b7bd31e2,
		0x27e11c91b5f24e7c,
		0x28376eda6bfc1835,
		0x155455c3e5071d85,
	}
	osswuMapB = fp{
		0xfb996971fe22a1e0,
		0x9aa93eb35b742d6f,
		0x8c476013de99c5c4,
		0x873e27c3a221e571,
		0xca72b5e45a52d888,
		0x06824061418a386b,
	}
	osswuMapC1 = fp{
		0xee7fbfffffffeaaa,
		0x07aaffffac54ffff,
		0xd9cc34a83dac3d89,
		0xd91dd2e13ce144af,
		0x92c6e9ed90d2eb35,
		0x0680447a8e5ff9a6,
	}
	osswuMapC2 = fp{
		0x43b571cad3215f1f,
		0xccb460ef1c702dc2,
		0x742d884f4f97100b,
		0xdb2c3e3238a3382b,
		0xe40f3fa13fce8f88,
		0x0073a2af9892a2ff,
	}
	oswwuMapZ = fp{
		0x886c00000023ffdc,
		0x0f70008d3090001d,
		0x77672417ed5828c3,
		0x9dac23e943dc1740,
		0x50553f1b9c131521,
		0x078c712fbe0ab6e8,
	}
	oswwuMapXd1  = *((&fp{}).Mul(&oswwuMapZ, &osswuMapA))
	negOsswuMapA = *(&fp{}).Neg(&osswuMapA)

	g1IsoXNum = []fp{
		{
			0x4d18b6f3af00131c,
			0x19fa219793fee28c,
			0x3f2885f1467f19ae,
			0x23dcea34f2ffb304,
			0xd15b58d2ffc00054,
			0x0913be200a20bef4,
		},
		{
			0x898985385cdbbd8b,
			0x3c79e43cc7d966aa,
			0x1597e193f4cd233a,
			0x8637ef1e4d6623ad,
			0x11b22deed20d827b,
			0x07097bc5998784ad,
		},
		{
			0xa542583a480b664b,
			0xfc7169c026e568c6,
			0x5ba2ef314ed8b5a6,
			0x5b5491c05102f0e7,
			0xdf6e99707d2a0079,
			0x0784151ed7605524,
		},
		{
			0x494e212870f72741,
			0xab9be52fbda43021,
			0x26f5577994e34c3d,
			0x049dfee82aefbd60,
			0x65dadd7828505289,
			0x0e93d431ea011aeb,
		},
		{
			0x90ee774bd6a74d45,
			0x7ada1c8a41bfb185,
			0x0f1a8953b325f464,
			0x104c24211be4805c,
			0x169139d319ea7a8f,
			0x09f20ead8e532bf6,
		},
		{
			0x6ddd93e2f43626b7,
			0xa5482c9aa1ccd7bd,
			0x143245631883f4bd,
			0x2e0a94ccf77ec0db,
			0xb0282d480e56489f,
			0x18f4bfcbb4368929,
		},
		{
			0x23c5f0c953402dfd,
			0x7a43ff6958ce4fe9,
			0x2c390d3d2da5df63,
			0xd0df5c98e1f9d70f,
			0xffd89869a572b297,
			0x1277ffc72f25e8fe,
		},
		{
			0x79f4f0490f06a8a6,
			0x85f894a88030fd81,
			0x12da3054b18b6410,
			0xe2a57f6505880d65,
			0xbba074f260e400f1,
			0x08b76279f621d028,
		},
		{
			0xe67245ba78d5b00b,
			0x8456ba9a1f186475,
			0x7888bff6e6b33bb4,
			0xe21585b9a30f86cb,
			0x05a69cdcef55feee,
			0x09e699dd9adfa5ac,
		},
		{
			0x0de5c357bff57107,
			0x0a0db4ae6b1a10b2,
			0xe256bb67b3b3cd8d,
			0x8ad456574e9db24f,
			0x0443915f50fd4179,
			0x098c4bf7de8b6375,
		},
		{
			0xe6b0617e7dd929c7,
			0xfe6e37d442537375,
			0x1dafdeda137a489e,
			0xe4efd1ad3f767ceb,
			0x4a51d8667f0fe1cf,
			0x054fdf4bbf1d821c,
		},
		{
			0x72db2a50658d767b,
			0x8abf91faa257b3d5,
			0xe969d6833764ab47,
			0x464170142a1009eb,
			0xb14f01aadb30be2f,
			0x18ae6a856f40715d,
		},
	}
	g1IsoXDen = []fp{
		{
			0xb962a077fdb0f945,
			0xa6a9740fefda13a0,
			0xc14d568c3ed6c544,
			0xb43fc37b908b133e,
			0x9c0b3ac929599016,
			0x0165aa6c93ad115f,
		},
		{
			0x23279a3ba506c1d9,
			0x92cfca0a9465176a,
			0x3b294ab13755f0ff,
			0x116dda1c5070ae93,
			0xed4530924cec2045,
			0x083383d6ed81f1ce,
		},
		{
			0x9885c2a6449fecfc,
			0x4a2b54ccd37733f0,
			0x17da9ffd8738c142,
			0xa0fba72732b3fafd,
			0xff364f36e54b6812,
			0x0f29c13c660523e2,
		},
		{
			0xe349cc118278f041,
			0xd487228f2f3204fb,
			0xc9d325849ade5150,
			0x43a92bd69c15c2df,
			0x1c2c7844bc417be4,
			0x12025184f407440c,
		},
		{
			0x587f65ae6acb057b,
			0x1444ef325140201f,
			0xfbf995e71270da49,
			0xccda066072436a42,
			0x7408904f0f186bb2,
			0x13b93c63edf6c015,
		},
		{
			0xfb918622cd141920,
			0x4a4c64423ecaddb4,
			0x0beb232927f7fb26,
			0x30f94df6f83a3dc2,
			0xaeedd424d780f388,
			0x06cc402dd594bbeb,
		},
		{
			0xd41f761151b23f8f,
			0x32a92465435719b3,
			0x64f436e888c62cb9,
			0xdf70a9a1f757c6e4,
			0x6933a38d5b594c81,
			0x0c6f7f7237b46606,
		},
		{
			0x693c08747876c8f7,
			0x22c9850bf9cf80f0,
			0x8e9071dab950c124,
			0x89bc62d61c7baf23,
			0xbc6be2d8dad57c23,
			0x17916987aa14a122,
		},
		{
			0x1be3ff439c1316fd,
			0x9965243a7571dfa7,
			0xc7f7f62962f5cd81,
			0x32c6aa9af394361c,
			0xbbc2ee18e1c227f4,
			0x0c102cbac531bb34,
		},
		{
			0x997614c97bacbf07,
			0x61f86372b99192c0,
			0x5b8c95fc14353fc3,
			0xca2b066c2a87492f,
			0x16178f5bbf698711,
			0x12a6dcd7f0f4e0e8,
		},
		{
			0x760900000002fffd,
			0xebf4000bc40c0002,
			0x5f48985753c758ba,
			0x77ce585370525745,
			0x5c071a97a256ec6d,
			0x15f65ec3fa80e493,
		},
	}
	g1IsoYNum = []fp{
		{
			0x2b567ff3e2837267,
			0x1d4d9e57b958a767,
			0xce028fea04bd7373,
			0xcc31a30a0b6cd3df,
			0x7d7b18a682692693,
			0x0d300744d42a0310,
		},
		{
			0x99c2555fa542493f,
			0xfe7f53cc4874f878,
			0x5df0608b8f97608a,
			0x14e03832052b49c8,
			0x706326a6957dd5a4,
			0x0a8dadd9c2414555,
		},
		{
			0x13d942922a5cf63a,
			0x357e33e36e261e7d,
			0xcf05a27c8456088d,
			0x0000bd1de7ba50f0,
			0x83d0c7532f8c1fde,
			0x13f70bf38bbf2905,
		},
		{
			0x5c57fd95bfafbdbb,
			0x28a359a65e541707,
			0x3983ceb4f6360b6d,
			0xafe19ff6f97e6d53,
			0xb3468f4550192bf7,
			0x0bb6cde49d8ba257,
		},
		{
			0x590b62c7ff8a513f,
			0x314b4ce372cacefd,
			0x6bef32ce94b8a800,
			0x6ddf84a095713d5f,
			0x64eace4cb0982191,
			0x0386213c651b888d,
		},
		{
			0xa5310a31111bbcdd,
			0xa14ac0f5da148982,
			0xf9ad9cc95423d2e9,
			0xaa6ec095283ee4a7,
			0xcf5b1f022e1c9107,
			0x01fddf5aed881793,
		},
		{
			0x65a572b0d7a7d950,
			0xe25c2d8183473a19,
			0xc2fcebe7cb877dbd,
			0x05b2d36c769a89b0,
			0xba12961be86e9efb,
			0x07eb1b29c1dfde1f,
		},
		{
			0x93e09572f7c4cd24,
			0x364e929076795091,
			0x8569467e68af51b5,
			0xa47da89439f5340f,
			0xf4fa918082e44d64,
			0x0ad52ba3e6695a79,
		},
		{
			0x911429844e0d5f54,
			0xd03f51a3516bb233,
			0x3d587e5640536e66,
			0xfa86d2a3a9a73482,
			0xa90ed5adf1ed5537,
			0x149c9c326a5e7393,
		},
		{
			0x462bbeb03c12921a,
			0xdc9af5fa0a274a17,
			0x9a558ebde836ebed,
			0x649ef8f11a4fae46,
			0x8100e1652b3cdc62,
			0x1862bd62c291dacb,
		},
		{
			0x05c9b8ca89f12c26,
			0x0194160fa9b9ac4f,
			0x6a643d5a6879fa2c,
			0x14665bdd8846e19d,
			0xbb1d0d53af3ff6bf,
			0x12c7e1c3b28962e5,
		},
		{
			0xb55ebf900b8a3e17,
			0xfedc77ec1a9201c4,
			0x1f07db10ea1a4df4,
			0x0dfbd15dc41a594d,
			0x389547f2334a5391,
			0x02419f98165871a4,
		},
		{
			0xb416af000745fc20,
			0x8e563e9d1ea6d0f5,
			0x7c763e17763a0652,
			0x01458ef0159ebbef,
			0x8346fe421f96bb13,
			0x0d2d7b829ce324d2,
		},
		{
			0x93096bb538d64615,
			0x6f2a2619951d823a,
			0x8f66b3ea59514fa4,
			0xf563e63704f7092f,
			0x724b136c4cf2d9fa,
			0x046959cfcfd0bf49,
		},
		{
			0xea748d4b6e405346,
			0x91e9079c2c02d58f,
			0x41064965946d9b59,
			0xa06731f1d2bbe1ee,
			0x07f897e267a33f1b,
			0x1017290919210e5f,
		},
		{
			0x872aa6c17d985097,
			0xeecc53161264562a,
			0x07afe37afff55002,
			0x54759078e5be6838,
			0xc4b92d15db8acca8,
			0x106d87d1b51d13b9,
		},
	}
	g1IsoYDen = []fp{
		{
			0xeb6c359d47e52b1c,
			0x18ef5f8a10634d60,
			0xddfa71a0889d5b7e,
			0x723e71dcc5fc1323,
			0x52f45700b70d5c69,
			0x0a8b981ee47691f1,
		},
		{
			0x616a3c4f5535b9fb,
			0x6f5f037395dbd911,
			0xf25f4cc5e35c65da,
			0x3e50dffea3c62658,
			0x6a33dca523560776,
			0x0fadeff77b6bfe3e,
		},
		{
			0x2be9b66df470059c,
			0x24a2c159a3d36742,
			0x115dbe7ad10c2a37,
			0xb6634a652ee5884d,
			0x04fe8bb2b8d81af4,
			0x01c2a7a256fe9c41,
		},
		{
			0xf27bf8ef3b75a386,
			0x898b367476c9073f,
			0x24482e6b8c2f4e5f,
			0xc8e0bbd6fe110806,
			0x59b0c17f7631448a,
			0x11037cd58b3dbfbd,
		},
		{
			0x31c7912ea267eec6,
			0x1dbf6f1c5fcdb700,
			0xd30d4fe3ba86fdb1,
			0x3cae528fbee9a2a4,
			0xb1cce69b6aa9ad9a,
			0x044393bb632d94fb,
		},
		{
			0xc66ef6efeeb5c7e8,
			0x9824c289dd72bb55,
			0x71b1a4d2f119981d,
			0x104fc1aafb0919cc,
			0x0e49df01d942a628,
			0x096c3a09773272d4,
		},
		{
			0x9abc11eb5fadeff4,
			0x32dca50a885728f0,
			0xfb1fa3721569734c,
			0xc4b76271ea6506b3,
			0xd466a75599ce728e,
			0x0c81d4645f4cb6ed,
		},
		{
			0x4199f10e5b8be45b,
			0xda64e495b1e87930,
			0xcb353efe9b33e4ff,
			0x9e9efb24aa6424c6,
			0xf08d33680a237465,
			0x0d3378023e4c7406,
		},
		{
			0x7eb4ae92ec74d3a5,
			0xc341b4aa9fac3497,
			0x5be603899e907687,
			0x03bfd9cca75cbdeb,
			0x564c2935a96bfa93,
			0x0ef3c33371e2fdb5,
		},
		{
			0x7ee91fd449f6ac2e,
			0xe5d5bd5cb9357a30,
			0x773a8ca5196b1380,
			0xd0fda172174ed023,
			0x6cb95e0fa776aead,
			0x0d22d5a40cec7cff,
		},
		{
			0xf727e09285fd8519,
			0xdc9d55a83017897b,
			0x7549d8bd057894ae,
			0x178419613d90d8f8,
			0xfce95ebdeb5b490a,
			0x0467ffaef23fc49e,
		},
		{
			0xc1769e6a7c385f1b,
			0x79bc930deac01c03,
			0x5461c75a23ede3b5,
			0x6e20829e5c230c45,
			0x828e0f1e772a53cd,
			0x116aefa749127bff,
		},
		{
			0x101c10bf2744c10a,
			0xbbf18d053a6a3154,
			0xa0ecf39ef026f602,
			0xfc009d4996dc5153,
			0xb9000209d5bd08d3,
			0x189e5fe4470cd73c,
		},
		{
			0x7ebd546ca1575ed2,
			0xe47d5a981d081b55,
			0x57b2b625b6d4ca21,
			0xb0a1ba04228520cc,
			0x98738983c2107ff3,
			0x13dddbc4799d81d6,
		},
		{
			0x09319f2e39834935,
			0x039e952cbdb05c21,
			0x55ba77a9a2f76493,
			0xfd04e3dfc6086467,
			0xfb95832e7d78742e,
			0x0ef9c24eccaf5e0e,
		},
		{
			0x760900000002fffd,
			0xebf4000bc40c0002,
			0x5f48985753c758ba,
			0x77ce585370525745,
			0x5c071a97a256ec6d,
			0x15f65ec3fa80e493,
		},
	}
)

// G1 is a point in g1
type G1 struct {
	x, y, z fp
}

// Random creates a random point on the curve
// from the specified reader
func (g1 *G1) Random(reader io.Reader) (*G1, error) {
	var seed [native.WideFieldBytes]byte
	n, err := reader.Read(seed[:])

	if err != nil {
		return nil, errors.Wrap(err, "random could not read from stream")
	}
	if n != native.WideFieldBytes {
		return nil, fmt.Errorf("insufficient bytes read %d when %d are needed", n, WideFieldBytes)
	}
	dst := []byte("BLS12381G1_XMD:SHA-256_SSWU_RO_")
	return g1.Hash(native.EllipticPointHasherSha256(), seed[:], dst), nil
}

// Hash uses the hasher to map bytes to a valid point
func (g1 *G1) Hash(hash *native.EllipticPointHasher, msg, dst []byte) *G1 {
	var u []byte
	var u0, u1 fp
	var r0, r1, q0, q1 G1

	switch hash.Type() {
	case native.XMD:
		u = native.ExpandMsgXmd(hash, msg, dst, 128)
	case native.XOF:
		u = native.ExpandMsgXof(hash, msg, dst, 128)
	}

	var buf [WideFieldBytes]byte
	copy(buf[:64], internal.ReverseScalarBytes(u[:64]))
	u0.SetBytesWide(&buf)
	copy(buf[:64], internal.ReverseScalarBytes(u[64:]))
	u1.SetBytesWide(&buf)

	r0.osswu3mod4(&u0)
	r1.osswu3mod4(&u1)
	q0.isogenyMap(&r0)
	q1.isogenyMap(&r1)
	g1.Add(&q0, &q1)
	return g1.ClearCofactor(g1)
}

// Identity returns the identity point
func (g1 *G1) Identity() *G1 {
	g1.x.SetZero()
	g1.y.SetOne()
	g1.z.SetZero()
	return g1
}

// Generator returns the base point
func (g1 *G1) Generator() *G1 {
	g1.x.Set(&g1x)
	g1.y.Set(&g1y)
	g1.z.SetOne()
	return g1
}

// IsIdentity returns true if this point is at infinity
func (g1 *G1) IsIdentity() int {
	return g1.z.IsZero()
}

// IsOnCurve determines if this point represents a valid curve point
func (g1 *G1) IsOnCurve() int {
	// Y^2 Z = X^3 + b Z^3
	var lhs, rhs, t fp
	lhs.Square(&g1.y)
	lhs.Mul(&lhs, &g1.z)

	rhs.Square(&g1.x)
	rhs.Mul(&rhs, &g1.x)
	t.Square(&g1.z)
	t.Mul(&t, &g1.z)
	t.Mul(&t, &curveG1B)
	rhs.Add(&rhs, &t)

	return lhs.Equal(&rhs)
}

// InCorrectSubgroup returns 1 if the point is torsion free, 0 otherwise
func (g1 *G1) InCorrectSubgroup() int {
	var t G1
	t.multiply(g1, &fqModulusBytes)
	return t.IsIdentity()
}

// Add adds this point to another point.
func (g1 *G1) Add(arg1, arg2 *G1) *G1 {
	// Algorithm 7, https://eprint.iacr.org/2015/1060.pdf
	var t0, t1, t2, t3, t4, x3, y3, z3 fp

	t0.Mul(&arg1.x, &arg2.x)
	t1.Mul(&arg1.y, &arg2.y)
	t2.Mul(&arg1.z, &arg2.z)
	t3.Add(&arg1.x, &arg1.y)
	t4.Add(&arg2.x, &arg2.y)
	t3.Mul(&t3, &t4)
	t4.Add(&t0, &t1)
	t3.Sub(&t3, &t4)
	t4.Add(&arg1.y, &arg1.z)
	x3.Add(&arg2.y, &arg2.z)
	t4.Mul(&t4, &x3)
	x3.Add(&t1, &t2)
	t4.Sub(&t4, &x3)
	x3.Add(&arg1.x, &arg1.z)
	y3.Add(&arg2.x, &arg2.z)
	x3.Mul(&x3, &y3)
	y3.Add(&t0, &t2)
	y3.Sub(&x3, &y3)
	x3.Double(&t0)
	t0.Add(&t0, &x3)
	t2.MulBy3b(&t2)
	z3.Add(&t1, &t2)
	t1.Sub(&t1, &t2)
	y3.MulBy3b(&y3)
	x3.Mul(&t4, &y3)
	t2.Mul(&t3, &t1)
	x3.Sub(&t2, &x3)
	y3.Mul(&y3, &t0)
	t1.Mul(&t1, &z3)
	y3.Add(&t1, &y3)
	t0.Mul(&t0, &t3)
	z3.Mul(&z3, &t4)
	z3.Add(&z3, &t0)

	g1.x.Set(&x3)
	g1.y.Set(&y3)
	g1.z.Set(&z3)
	return g1
}

// Sub subtracts the two points
func (g1 *G1) Sub(arg1, arg2 *G1) *G1 {
	var t G1
	t.Neg(arg2)
	return g1.Add(arg1, &t)
}

// Double this point
func (g1 *G1) Double(a *G1) *G1 {
	// Algorithm 9, https://eprint.iacr.org/2015/1060.pdf
	var t0, t1, t2, x3, y3, z3 fp

	t0.Square(&a.y)
	z3.Double(&t0)
	z3.Double(&z3)
	z3.Double(&z3)
	t1.Mul(&a.y, &a.z)
	t2.Square(&a.z)
	t2.MulBy3b(&t2)
	x3.Mul(&t2, &z3)
	y3.Add(&t0, &t2)
	z3.Mul(&t1, &z3)
	t1.Double(&t2)
	t2.Add(&t2, &t1)
	t0.Sub(&t0, &t2)
	y3.Mul(&t0, &y3)
	y3.Add(&y3, &x3)
	t1.Mul(&a.x, &a.y)
	x3.Mul(&t0, &t1)
	x3.Double(&x3)

	e := a.IsIdentity()
	g1.x.CMove(&x3, t0.SetZero(), e)
	g1.z.CMove(&z3, &t0, e)
	g1.y.CMove(&y3, t0.SetOne(), e)
	return g1
}

// Mul multiplies this point by the input scalar
func (g1 *G1) Mul(a *G1, s *native.Field) *G1 {
	bytes := s.Bytes()
	return g1.multiply(a, &bytes)
}

func (g1 *G1) multiply(a *G1, bytes *[native.FieldBytes]byte) *G1 {
	var p G1
	precomputed := [16]*G1{}
	precomputed[0] = new(G1).Identity()
	precomputed[1] = new(G1).Set(a)
	for i := 2; i < 16; i += 2 {
		precomputed[i] = new(G1).Double(precomputed[i>>1])
		precomputed[i+1] = new(G1).Add(precomputed[i], a)
	}
	p.Identity()
	for i := 0; i < 256; i += 4 {
		// Brouwer / windowing method. window size of 4.
		for j := 0; j < 4; j++ {
			p.Double(&p)
		}
		window := bytes[32-1-i>>3] >> (4 - i&0x04) & 0x0F
		p.Add(&p, precomputed[window])
	}
	return g1.Set(&p)
}

// MulByX multiplies by BLS X using double and add
func (g1 *G1) MulByX(a *G1) *G1 {
	// Skip first bit since its always zero
	var s, t, r G1
	r.Identity()
	t.Set(a)

	for x := paramX >> 1; x != 0; x >>= 1 {
		t.Double(&t)
		s.Add(&r, &t)
		r.CMove(&r, &s, int(x&1))
	}
	// Since BLS_X is negative, flip the sign
	return g1.Neg(&r)
}

// ClearCofactor multiplies by (1 - z), where z is the parameter of BLS12-381, which
// [suffices to clear](https://ia.cr/2019/403) the cofactor and map
// elliptic curve points to elements of G1.
func (g1 *G1) ClearCofactor(a *G1) *G1 {
	var t G1
	t.MulByX(a)
	return g1.Sub(a, &t)
}

// Neg negates this point
func (g1 *G1) Neg(a *G1) *G1 {
	g1.Set(a)
	g1.y.CNeg(&a.y, -(a.IsIdentity() - 1))
	return g1
}

// Set copies a into g1
func (g1 *G1) Set(a *G1) *G1 {
	g1.x.Set(&a.x)
	g1.y.Set(&a.y)
	g1.z.Set(&a.z)
	return g1
}

// BigInt returns the x and y as big.Ints in affine
func (g1 *G1) BigInt() (x, y *big.Int) {
	var t G1
	t.ToAffine(g1)
	x = t.x.BigInt()
	y = t.y.BigInt()
	return
}

// SetBigInt creates a point from affine x, y
// and returns the point if it is on the curve
func (g1 *G1) SetBigInt(x, y *big.Int) (*G1, error) {
	var xx, yy fp
	var pp G1
	pp.x = *(xx.SetBigInt(x))
	pp.y = *(yy.SetBigInt(y))

	if pp.x.IsZero()&pp.y.IsZero() == 1 {
		pp.Identity()
		return g1.Set(&pp), nil
	}

	pp.z.SetOne()

	// If not the identity point and not on the curve then invalid
	if (pp.IsOnCurve()&pp.InCorrectSubgroup())|(xx.IsZero()&yy.IsZero()) == 0 {
		return nil, fmt.Errorf("invalid coordinates")
	}
	return g1.Set(&pp), nil
}

// ToCompressed serializes this element into compressed form.
func (g1 *G1) ToCompressed() [FieldBytes]byte {
	var out [FieldBytes]byte
	var t G1
	t.ToAffine(g1)
	xBytes := t.x.Bytes()
	copy(out[:], internal.ReverseScalarBytes(xBytes[:]))
	isInfinity := byte(g1.IsIdentity())
	// Compressed flag
	out[0] |= 1 << 7
	// Is infinity
	out[0] |= (1 << 6) & -isInfinity
	// Sign of y only set if not infinity
	out[0] |= (byte(t.y.LexicographicallyLargest()) << 5) & (isInfinity - 1)
	return out
}

// FromCompressed deserializes this element from compressed form.
func (g1 *G1) FromCompressed(input *[FieldBytes]byte) (*G1, error) {
	var xFp, yFp fp
	var x [FieldBytes]byte
	var p G1
	compressedFlag := int((input[0] >> 7) & 1)
	infinityFlag := int((input[0] >> 6) & 1)
	sortFlag := int((input[0] >> 5) & 1)

	if compressedFlag != 1 {
		return nil, errors.New("compressed flag must be set")
	}

	if infinityFlag == 1 {
		return g1.Identity(), nil
	}

	copy(x[:], internal.ReverseScalarBytes(input[:]))
	// Mask away the flag bits
	x[FieldBytes-1] &= 0x1F
	_, valid := xFp.SetBytes(&x)

	if valid != 1 {
		return nil, errors.New("invalid bytes - not in field")
	}

	yFp.Square(&xFp)
	yFp.Mul(&yFp, &xFp)
	yFp.Add(&yFp, &curveG1B)

	_, wasSquare := yFp.Sqrt(&yFp)
	if wasSquare != 1 {
		return nil, errors.New("point is not on the curve")
	}

	yFp.CNeg(&yFp, yFp.LexicographicallyLargest()^sortFlag)
	p.x.Set(&xFp)
	p.y.Set(&yFp)
	p.z.SetOne()
	if p.InCorrectSubgroup() == 0 {
		return nil, errors.New("point is not in correct subgroup")
	}
	return g1.Set(&p), nil
}

// ToUncompressed serializes this element into uncompressed form.
func (g1 *G1) ToUncompressed() [WideFieldBytes]byte {
	var out [WideFieldBytes]byte
	var t G1
	t.ToAffine(g1)
	xBytes := t.x.Bytes()
	yBytes := t.y.Bytes()
	copy(out[:FieldBytes], internal.ReverseScalarBytes(xBytes[:]))
	copy(out[FieldBytes:], internal.ReverseScalarBytes(yBytes[:]))
	isInfinity := byte(g1.IsIdentity())
	out[0] |= (1 << 6) & -isInfinity
	return out
}

// FromUncompressed deserializes this element from uncompressed form.
func (g1 *G1) FromUncompressed(input *[WideFieldBytes]byte) (*G1, error) {
	var xFp, yFp fp
	var t [FieldBytes]byte
	var p G1
	infinityFlag := int((input[0] >> 6) & 1)

	if infinityFlag == 1 {
		return g1.Identity(), nil
	}

	copy(t[:], internal.ReverseScalarBytes(input[:FieldBytes]))
	// Mask away top bits
	t[FieldBytes-1] &= 0x1F

	_, valid := xFp.SetBytes(&t)
	if valid == 0 {
		return nil, errors.New("invalid bytes - x not in field")
	}
	copy(t[:], internal.ReverseScalarBytes(input[FieldBytes:]))
	_, valid = yFp.SetBytes(&t)
	if valid == 0 {
		return nil, errors.New("invalid bytes - y not in field")
	}

	p.x.Set(&xFp)
	p.y.Set(&yFp)
	p.z.SetOne()

	if p.IsOnCurve() == 0 {
		return nil, errors.New("point is not on the curve")
	}
	if p.InCorrectSubgroup() == 0 {
		return nil, errors.New("point is not in correct subgroup")
	}
	return g1.Set(&p), nil
}

// ToAffine converts the point into affine coordinates
func (g1 *G1) ToAffine(a *G1) *G1 {
	var wasInverted int
	var zero, x, y, z fp
	_, wasInverted = z.Invert(&a.z)
	x.Mul(&a.x, &z)
	y.Mul(&a.y, &z)

	g1.x.CMove(&zero, &x, wasInverted)
	g1.y.CMove(&zero, &y, wasInverted)
	g1.z.CMove(&zero, z.SetOne(), wasInverted)
	return g1
}

// GetX returns the affine X coordinate
func (g1 *G1) GetX() *fp {
	var t G1
	t.ToAffine(g1)
	return &t.x
}

// GetY returns the affine Y coordinate
func (g1 *G1) GetY() *fp {
	var t G1
	t.ToAffine(g1)
	return &t.y
}

// Equal returns 1 if the two points are equal 0 otherwise.
func (g1 *G1) Equal(rhs *G1) int {
	var x1, x2, y1, y2 fp
	var e1, e2 int

	// This technique avoids inversions
	x1.Mul(&g1.x, &rhs.z)
	x2.Mul(&rhs.x, &g1.z)

	y1.Mul(&g1.y, &rhs.z)
	y2.Mul(&rhs.y, &g1.z)

	e1 = g1.z.IsZero()
	e2 = rhs.z.IsZero()

	// Both at infinity or coordinates are the same
	return (e1 & e2) | (^e1 & ^e2)&x1.Equal(&x2)&y1.Equal(&y2)
}

// CMove sets g1 = arg1 if choice == 0 and g1 = arg2 if choice == 1
func (g1 *G1) CMove(arg1, arg2 *G1, choice int) *G1 {
	g1.x.CMove(&arg1.x, &arg2.x, choice)
	g1.y.CMove(&arg1.y, &arg2.y, choice)
	g1.z.CMove(&arg1.z, &arg2.z, choice)
	return g1
}

// SumOfProducts computes the multi-exponentiation for the specified
// points and scalars and stores the result in `g1`.
// Returns an error if the lengths of the arguments is not equal.
func (g1 *G1) SumOfProducts(points []*G1, scalars []*native.Field) (*G1, error) {
	const Upper = 256
	const W = 4
	const Windows = Upper / W // careful--use ceiling division in case this doesn't divide evenly
	var sum G1
	if len(points) != len(scalars) {
		return nil, fmt.Errorf("length mismatch")
	}

	bucketSize := 1 << W
	windows := make([]G1, Windows)
	bytes := make([][32]byte, len(scalars))
	buckets := make([]G1, bucketSize)

	for i := 0; i < len(windows); i++ {
		windows[i].Identity()
	}

	for i, scalar := range scalars {
		bytes[i] = scalar.Bytes()
	}

	for j := 0; j < len(windows); j++ {
		for i := 0; i < bucketSize; i++ {
			buckets[i].Identity()
		}

		for i := 0; i < len(scalars); i++ {
			// j*W to get the nibble
			// >> 3 to convert to byte, / 8
			// (W * j & W) gets the nibble, mod W
			// 1 << W - 1 to get the offset
			index := bytes[i][j*W>>3] >> (W * j & W) & (1<<W - 1) // little-endian
			buckets[index].Add(&buckets[index], points[i])
		}

		sum.Identity()

		for i := bucketSize - 1; i > 0; i-- {
			sum.Add(&sum, &buckets[i])
			windows[j].Add(&windows[j], &sum)
		}
	}

	g1.Identity()
	for i := len(windows) - 1; i >= 0; i-- {
		for j := 0; j < W; j++ {
			g1.Double(g1)
		}

		g1.Add(g1, &windows[i])
	}
	return g1, nil
}

func (g1 *G1) osswu3mod4(u *fp) *G1 {
	// Taken from section 8.8.1 in
	// <https://www.ietf.org/archive/id/draft-irtf-cfrg-hash-to-curve-10.html>
	var tv1, tv2, tv3, tv4, xd, x1n, x2n, gxd, gx1, y1, y2 fp

	// tv1 = u^2
	tv1.Square(u)
	// tv3 = Z * tv1
	tv3.Mul(&oswwuMapZ, &tv1)
	// tv2 = tv3^2
	tv2.Square(&tv3)
	// xd = tv2 + tv3
	xd.Add(&tv2, &tv3)
	// x1n = xd + 1
	x1n.Add(&xd, &r)
	// x1n = x1n * B
	x1n.Mul(&x1n, &osswuMapB)
	// xd = -A * xd
	xd.Mul(&negOsswuMapA, &xd)
	// xd = CMOV(xd, Z * A, xd == 0)
	xd.CMove(&xd, &oswwuMapXd1, xd.IsZero())
	// tv2 = xd^2
	tv2.Square(&xd)

	gxd.Mul(&tv2, &xd)
	tv2.Mul(&tv2, &osswuMapA)
	gx1.Square(&x1n)
	gx1.Add(&gx1, &tv2)
	gx1.Mul(&gx1, &x1n)
	tv2.Mul(&osswuMapB, &gxd)

	gx1.Add(&gx1, &tv2)
	tv4.Square(&gxd)
	tv2.Mul(&gx1, &gxd)
	tv4.Mul(&tv4, &tv2)
	y1.pow(&tv4, &osswuMapC1)
	y1.Mul(&y1, &tv2)

	x2n.Mul(&tv3, &x1n)
	y2.Mul(&y1, &osswuMapC2)
	y2.Mul(&y2, &tv1)
	y2.Mul(&y2, u)

	tv2.Square(&y1)
	tv2.Mul(&tv2, &gxd)
	e2 := tv2.Equal(&gx1)

	x2n.CMove(&x2n, &x1n, e2)
	y2.CMove(&y2, &y1, e2)

	e3 := u.Sgn0() ^ y2.Sgn0()
	y2.CNeg(&y2, e3)

	g1.z.SetOne()
	g1.y.Set(&y2)
	_, _ = g1.x.Invert(&xd)
	g1.x.Mul(&g1.x, &x2n)
	return g1
}

func (g1 *G1) isogenyMap(a *G1) *G1 {
	const Degree = 16
	var xs [Degree]fp
	xs[0] = r
	xs[1].Set(&a.x)
	xs[2].Square(&a.x)
	for i := 3; i < Degree; i++ {
		xs[i].Mul(&xs[i-1], &a.x)
	}

	xNum := computeKFp(xs[:], g1IsoXNum)
	xDen := computeKFp(xs[:], g1IsoXDen)
	yNum := computeKFp(xs[:], g1IsoYNum)
	yDen := computeKFp(xs[:], g1IsoYDen)

	g1.x.Invert(&xDen)
	g1.x.Mul(&g1.x, &xNum)

	g1.y.Invert(&yDen)
	g1.y.Mul(&g1.y, &yNum)
	g1.y.Mul(&g1.y, &a.y)
	g1.z.SetOne()
	return g1
}

func computeKFp(xxs []fp, k []fp) fp {
	var xx, t fp
	for i := range k {
		xx.Add(&xx, t.Mul(&xxs[i], &k[i]))
	}
	return xx
}
