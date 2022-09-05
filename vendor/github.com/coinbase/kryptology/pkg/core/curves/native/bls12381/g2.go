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
	g2x = fp2{
		A: fp{
			0xf5f2_8fa2_0294_0a10,
			0xb3f5_fb26_87b4_961a,
			0xa1a8_93b5_3e2a_e580,
			0x9894_999d_1a3c_aee9,
			0x6f67_b763_1863_366b,
			0x0581_9192_4350_bcd7,
		},
		B: fp{
			0xa5a9_c075_9e23_f606,
			0xaaa0_c59d_bccd_60c3,
			0x3bb1_7e18_e286_7806,
			0x1b1a_b6cc_8541_b367,
			0xc2b6_ed0e_f215_8547,
			0x1192_2a09_7360_edf3,
		},
	}
	g2y = fp2{
		A: fp{
			0x4c73_0af8_6049_4c4a,
			0x597c_fa1f_5e36_9c5a,
			0xe7e6_856c_aa0a_635a,
			0xbbef_b5e9_6e0d_495f,
			0x07d3_a975_f0ef_25a2,
			0x0083_fd8e_7e80_dae5,
		},
		B: fp{
			0xadc0_fc92_df64_b05d,
			0x18aa_270a_2b14_61dc,
			0x86ad_ac6a_3be4_eba0,
			0x7949_5c4e_c93d_a33a,
			0xe717_5850_a43c_caed,
			0x0b2b_c2a1_63de_1bf2,
		},
	}
	curveG2B = fp2{
		A: fp{
			0xaa27_0000_000c_fff3,
			0x53cc_0032_fc34_000a,
			0x478f_e97a_6b0a_807f,
			0xb1d3_7ebe_e6ba_24d7,
			0x8ec9_733b_bf78_ab2f,
			0x09d6_4551_3d83_de7e,
		},
		B: fp{
			0xaa27_0000_000c_fff3,
			0x53cc_0032_fc34_000a,
			0x478f_e97a_6b0a_807f,
			0xb1d3_7ebe_e6ba_24d7,
			0x8ec9_733b_bf78_ab2f,
			0x09d6_4551_3d83_de7e,
		},
	}
	curveG23B = fp2{
		A: fp{
			0x447600000027552e,
			0xdcb8009a43480020,
			0x6f7ee9ce4a6e8b59,
			0xb10330b7c0a95bc6,
			0x6140b1fcfb1e54b7,
			0x0381be097f0bb4e1,
		},
		B: fp{
			0x447600000027552e,
			0xdcb8009a43480020,
			0x6f7ee9ce4a6e8b59,
			0xb10330b7c0a95bc6,
			0x6140b1fcfb1e54b7,
			0x0381be097f0bb4e1,
		},
	}
	sswuMapA = fp2{
		A: fp{},
		B: fp{
			0xe53a000003135242,
			0x01080c0fdef80285,
			0xe7889edbe340f6bd,
			0x0b51375126310601,
			0x02d6985717c744ab,
			0x1220b4e979ea5467,
		},
	}
	sswuMapB = fp2{
		A: fp{
			0x22ea00000cf89db2,
			0x6ec832df71380aa4,
			0x6e1b94403db5a66e,
			0x75bf3c53a79473ba,
			0x3dd3a569412c0a34,
			0x125cdb5e74dc4fd1,
		},
		B: fp{
			0x22ea00000cf89db2,
			0x6ec832df71380aa4,
			0x6e1b94403db5a66e,
			0x75bf3c53a79473ba,
			0x3dd3a569412c0a34,
			0x125cdb5e74dc4fd1,
		},
	}
	sswuMapZ = fp2{
		A: fp{
			0x87ebfffffff9555c,
			0x656fffe5da8ffffa,
			0x0fd0749345d33ad2,
			0xd951e663066576f4,
			0xde291a3d41e980d3,
			0x0815664c7dfe040d,
		},
		B: fp{
			0x43f5fffffffcaaae,
			0x32b7fff2ed47fffd,
			0x07e83a49a2e99d69,
			0xeca8f3318332bb7a,
			0xef148d1ea0f4c069,
			0x040ab3263eff0206,
		},
	}
	sswuMapZInv = fp2{
		A: fp{
			0xacd0000000011110,
			0x9dd9999dc88ccccd,
			0xb5ca2ac9b76352bf,
			0xf1b574bcf4bc90ce,
			0x42dab41f28a77081,
			0x132fc6ac14cd1e12,
		},
		B: fp{
			0xe396ffffffff2223,
			0x4fbf332fcd0d9998,
			0x0c4bbd3c1aff4cc4,
			0x6b9c91267926ca58,
			0x29ae4da6aef7f496,
			0x10692e942f195791,
		},
	}
	swwuMapMbDivA = fp2{
		A: fp{
			0x903c555555474fb3,
			0x5f98cc95ce451105,
			0x9f8e582eefe0fade,
			0xc68946b6aebbd062,
			0x467a4ad10ee6de53,
			0x0e7146f483e23a05,
		},
		B: fp{
			0x29c2aaaaaab85af8,
			0xbf133368e30eeefa,
			0xc7a27a7206cffb45,
			0x9dee04ce44c9425c,
			0x04a15ce53464ce83,
			0x0b8fcaf5b59dac95,
		},
	}

	g2IsoXNum = []fp2{
		{
			A: fp{
				0x47f671c71ce05e62,
				0x06dd57071206393e,
				0x7c80cd2af3fd71a2,
				0x048103ea9e6cd062,
				0xc54516acc8d037f6,
				0x13808f550920ea41,
			},
			B: fp{
				0x47f671c71ce05e62,
				0x06dd57071206393e,
				0x7c80cd2af3fd71a2,
				0x048103ea9e6cd062,
				0xc54516acc8d037f6,
				0x13808f550920ea41,
			},
		},
		{
			A: fp{
				0x0000000000000000,
				0x0000000000000000,
				0x0000000000000000,
				0x0000000000000000,
				0x0000000000000000,
				0x0000000000000000,
			},
			B: fp{
				0x5fe55555554c71d0,
				0x873fffdd236aaaa3,
				0x6a6b4619b26ef918,
				0x21c2888408874945,
				0x2836cda7028cabc5,
				0x0ac73310a7fd5abd,
			},
		},
		{
			A: fp{
				0x0a0c5555555971c3,
				0xdb0c00101f9eaaae,
				0xb1fb2f941d797997,
				0xd3960742ef416e1c,
				0xb70040e2c20556f4,
				0x149d7861e581393b,
			},
			B: fp{
				0xaff2aaaaaaa638e8,
				0x439fffee91b55551,
				0xb535a30cd9377c8c,
				0x90e144420443a4a2,
				0x941b66d3814655e2,
				0x0563998853fead5e,
			},
		},
		{
			A: fp{
				0x40aac71c71c725ed,
				0x190955557a84e38e,
				0xd817050a8f41abc3,
				0xd86485d4c87f6fb1,
				0x696eb479f885d059,
				0x198e1a74328002d2,
			},
			B: fp{
				0x0000000000000000,
				0x0000000000000000,
				0x0000000000000000,
				0x0000000000000000,
				0x0000000000000000,
				0x0000000000000000,
			},
		},
	}
	g2IsoXDen = []fp2{
		{
			A: fp{
				0x0000000000000000,
				0x0000000000000000,
				0x0000000000000000,
				0x0000000000000000,
				0x0000000000000000,
				0x0000000000000000,
			},
			B: fp{
				0x1f3affffff13ab97,
				0xf25bfc611da3ff3e,
				0xca3757cb3819b208,
				0x3e6427366f8cec18,
				0x03977bc86095b089,
				0x04f69db13f39a952,
			},
		},
		{
			A: fp{
				0x447600000027552e,
				0xdcb8009a43480020,
				0x6f7ee9ce4a6e8b59,
				0xb10330b7c0a95bc6,
				0x6140b1fcfb1e54b7,
				0x0381be097f0bb4e1,
			},
			B: fp{
				0x7588ffffffd8557d,
				0x41f3ff646e0bffdf,
				0xf7b1e8d2ac426aca,
				0xb3741acd32dbb6f8,
				0xe9daf5b9482d581f,
				0x167f53e0ba7431b8,
			},
		},
		{
			A: fp{
				0x760900000002fffd,
				0xebf4000bc40c0002,
				0x5f48985753c758ba,
				0x77ce585370525745,
				0x5c071a97a256ec6d,
				0x15f65ec3fa80e493,
			},
			B: fp{
				0x0000000000000000,
				0x0000000000000000,
				0x0000000000000000,
				0x0000000000000000,
				0x0000000000000000,
				0x0000000000000000,
			},
		},
	}
	g2IsoYNum = []fp2{
		{
			A: fp{
				0x96d8f684bdfc77be,
				0xb530e4f43b66d0e2,
				0x184a88ff379652fd,
				0x57cb23ecfae804e1,
				0x0fd2e39eada3eba9,
				0x08c8055e31c5d5c3,
			},
			B: fp{
				0x96d8f684bdfc77be,
				0xb530e4f43b66d0e2,
				0x184a88ff379652fd,
				0x57cb23ecfae804e1,
				0x0fd2e39eada3eba9,
				0x08c8055e31c5d5c3,
			},
		},
		{
			A: fp{
				000000000000000000,
				0x0000000000000000,
				0x0000000000000000,
				0x0000000000000000,
				0x0000000000000000,
				0x0000000000000000,
			},
			B: fp{
				0xbf0a71c71c91b406,
				0x4d6d55d28b7638fd,
				0x9d82f98e5f205aee,
				0xa27aa27b1d1a18d5,
				0x02c3b2b2d2938e86,
				0x0c7d13420b09807f,
			},
		},
		{
			A: fp{
				0xd7f9555555531c74,
				0x21cffff748daaaa8,
				0x5a9ad1866c9bbe46,
				0x4870a2210221d251,
				0x4a0db369c0a32af1,
				0x02b1ccc429ff56af,
			},
			B: fp{
				0xe205aaaaaaac8e37,
				0xfcdc000768795556,
				0x0c96011a8a1537dd,
				0x1c06a963f163406e,
				0x010df44c82a881e6,
				0x174f45260f808feb,
			},
		},
		{
			A: fp{
				0xa470bda12f67f35c,
				0xc0fe38e23327b425,
				0xc9d3d0f2c6f0678d,
				0x1c55c9935b5a982e,
				0x27f6c0e2f0746764,
				0x117c5e6e28aa9054,
			},
			B: fp{
				0x0000000000000000,
				0x0000000000000000,
				0x0000000000000000,
				0x0000000000000000,
				0x0000000000000000,
				0x0000000000000000,
			},
		},
	}
	g2IsoYDen = []fp2{
		{
			A: fp{
				0x0162fffffa765adf,
				0x8f7bea480083fb75,
				0x561b3c2259e93611,
				0x11e19fc1a9c875d5,
				0xca713efc00367660,
				0x03c6a03d41da1151,
			},
			B: fp{
				0x0162fffffa765adf,
				0x8f7bea480083fb75,
				0x561b3c2259e93611,
				0x11e19fc1a9c875d5,
				0xca713efc00367660,
				0x03c6a03d41da1151,
			},
		},
		{
			A: fp{
				0x0000000000000000,
				0x0000000000000000,
				0x0000000000000000,
				0x0000000000000000,
				0x0000000000000000,
				0x0000000000000000,
			},
			B: fp{
				0x5db0fffffd3b02c5,
				0xd713f52358ebfdba,
				0x5ea60761a84d161a,
				0xbb2c75a34ea6c44a,
				0x0ac6735921c1119b,
				0x0ee3d913bdacfbf6,
			},
		},
		{
			A: fp{
				0x66b10000003affc5,
				0xcb1400e764ec0030,
				0xa73e5eb56fa5d106,
				0x8984c913a0fe09a9,
				0x11e10afb78ad7f13,
				0x05429d0e3e918f52,
			},
			B: fp{
				0x534dffffffc4aae6,
				0x5397ff174c67ffcf,
				0xbff273eb870b251d,
				0xdaf2827152870915,
				0x393a9cbaca9e2dc3,
				0x14be74dbfaee5748,
			},
		},
		{
			A: fp{
				0x760900000002fffd,
				0xebf4000bc40c0002,
				0x5f48985753c758ba,
				0x77ce585370525745,
				0x5c071a97a256ec6d,
				0x15f65ec3fa80e493,
			},
			B: fp{
				0x0000000000000000,
				0x0000000000000000,
				0x0000000000000000,
				0x0000000000000000,
				0x0000000000000000,
				0x0000000000000000,
			},
		},
	}

	// 1 / ((u+1) ^ ((q-1)/3))
	psiCoeffX = fp2{
		A: fp{},
		B: fp{
			0x890dc9e4867545c3,
			0x2af322533285a5d5,
			0x50880866309b7e2c,
			0xa20d1b8c7e881024,
			0x14e4f04fe2db9068,
			0x14e56d3f1564853a,
		},
	}
	// 1 / ((u+1) ^ (p-1)/2)
	psiCoeffY = fp2{
		A: fp{
			0x3e2f585da55c9ad1,
			0x4294213d86c18183,
			0x382844c88b623732,
			0x92ad2afd19103e18,
			0x1d794e4fac7cf0b9,
			0x0bd592fc7d825ec8,
		},
		B: fp{
			0x7bcfa7a25aa30fda,
			0xdc17dec12a927e7c,
			0x2f088dd86b4ebef1,
			0xd1ca2087da74d4a7,
			0x2da2596696cebc1d,
			0x0e2b7eedbbfd87d2,
		},
	}

	// 1 / 2 ^ ((q-1)/3)
	psi2CoeffX = fp2{
		A: fp{
			0xcd03c9e48671f071,
			0x5dab22461fcda5d2,
			0x587042afd3851b95,
			0x8eb60ebe01bacb9e,
			0x03f97d6e83d050d2,
			0x18f0206554638741,
		},
		B: fp{},
	}
)

// G2 is a point in g2
type G2 struct {
	x, y, z fp2
}

// Random creates a random point on the curve
// from the specified reader
func (g2 *G2) Random(reader io.Reader) (*G2, error) {
	var seed [native.WideFieldBytes]byte
	n, err := reader.Read(seed[:])

	if err != nil {
		return nil, errors.Wrap(err, "random could not read from stream")
	}
	if n != native.WideFieldBytes {
		return nil, fmt.Errorf("insufficient bytes read %d when %d are needed", n, WideFieldBytes)
	}
	dst := []byte("BLS12381G2_XMD:SHA-256_SSWU_RO_")
	return g2.Hash(native.EllipticPointHasherSha256(), seed[:], dst), nil
}

// Hash uses the hasher to map bytes to a valid point
func (g2 *G2) Hash(hash *native.EllipticPointHasher, msg, dst []byte) *G2 {
	var u []byte
	var u0, u1 fp2
	var r0, r1, q0, q1 G2

	switch hash.Type() {
	case native.XMD:
		u = native.ExpandMsgXmd(hash, msg, dst, 256)
	case native.XOF:
		u = native.ExpandMsgXof(hash, msg, dst, 256)
	}

	var buf [96]byte
	copy(buf[:64], internal.ReverseScalarBytes(u[:64]))
	u0.A.SetBytesWide(&buf)
	copy(buf[:64], internal.ReverseScalarBytes(u[64:128]))
	u0.B.SetBytesWide(&buf)
	copy(buf[:64], internal.ReverseScalarBytes(u[128:192]))
	u1.A.SetBytesWide(&buf)
	copy(buf[:64], internal.ReverseScalarBytes(u[192:]))
	u1.B.SetBytesWide(&buf)

	r0.sswu(&u0)
	r1.sswu(&u1)
	q0.isogenyMap(&r0)
	q1.isogenyMap(&r1)
	g2.Add(&q0, &q1)
	return g2.ClearCofactor(g2)
}

// Identity returns the identity point
func (g2 *G2) Identity() *G2 {
	g2.x.SetZero()
	g2.y.SetOne()
	g2.z.SetZero()
	return g2
}

// Generator returns the base point
func (g2 *G2) Generator() *G2 {
	g2.x.Set(&g2x)
	g2.y.Set(&g2y)
	g2.z.SetOne()
	return g2
}

// IsIdentity returns true if this point is at infinity
func (g2 *G2) IsIdentity() int {
	return g2.z.IsZero()
}

// IsOnCurve determines if this point represents a valid curve point
func (g2 *G2) IsOnCurve() int {
	// Y^2 Z = X^3 + b Z^3
	var lhs, rhs, t fp2
	lhs.Square(&g2.y)
	lhs.Mul(&lhs, &g2.z)

	rhs.Square(&g2.x)
	rhs.Mul(&rhs, &g2.x)
	t.Square(&g2.z)
	t.Mul(&t, &g2.z)
	t.Mul(&t, &curveG2B)
	rhs.Add(&rhs, &t)

	return lhs.Equal(&rhs)
}

// InCorrectSubgroup returns 1 if the point is torsion free, 0 otherwise
func (g2 *G2) InCorrectSubgroup() int {
	var t G2
	t.multiply(g2, &fqModulusBytes)
	return t.IsIdentity()
}

// Add adds this point to another point.
func (g2 *G2) Add(arg1, arg2 *G2) *G2 {
	// Algorithm 7, https://eprint.iacr.org/2015/1060.pdf
	var t0, t1, t2, t3, t4, x3, y3, z3 fp2

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

	g2.x.Set(&x3)
	g2.y.Set(&y3)
	g2.z.Set(&z3)
	return g2
}

// Sub subtracts the two points
func (g2 *G2) Sub(arg1, arg2 *G2) *G2 {
	var t G2
	t.Neg(arg2)
	return g2.Add(arg1, &t)
}

// Double this point
func (g2 *G2) Double(a *G2) *G2 {
	// Algorithm 9, https://eprint.iacr.org/2015/1060.pdf
	var t0, t1, t2, x3, y3, z3 fp2

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
	g2.x.CMove(&x3, t0.SetZero(), e)
	g2.z.CMove(&z3, &t0, e)
	g2.y.CMove(&y3, t0.SetOne(), e)
	return g2
}

// Mul multiplies this point by the input scalar
func (g2 *G2) Mul(a *G2, s *native.Field) *G2 {
	bytes := s.Bytes()
	return g2.multiply(a, &bytes)
}

func (g2 *G2) multiply(a *G2, bytes *[native.FieldBytes]byte) *G2 {
	var p G2
	precomputed := [16]*G2{}
	precomputed[0] = new(G2).Identity()
	precomputed[1] = new(G2).Set(a)
	for i := 2; i < 16; i += 2 {
		precomputed[i] = new(G2).Double(precomputed[i>>1])
		precomputed[i+1] = new(G2).Add(precomputed[i], a)
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
	return g2.Set(&p)
}

// MulByX multiplies by BLS X using double and add
func (g2 *G2) MulByX(a *G2) *G2 {
	// Skip first bit since its always zero
	var s, t, r G2
	r.Identity()
	t.Set(a)

	for x := paramX >> 1; x != 0; x >>= 1 {
		t.Double(&t)
		s.Add(&r, &t)
		r.CMove(&r, &s, int(x&1))
	}
	// Since BLS_X is negative, flip the sign
	return g2.Neg(&r)
}

// ClearCofactor using [Budroni-Pintore](https://ia.cr/2017/419).
// This is equivalent to multiplying by h_{eff} = 3(z^2 - 1) * h_2
// where h_2 is the cofactor of G_2 and z is the parameter of BLS12-381.
func (g2 *G2) ClearCofactor(a *G2) *G2 {
	var t1, t2, t3, pt G2

	t1.MulByX(a)
	t2.psi(a)

	pt.Double(a)
	pt.psi2(&pt)

	t3.Add(&t1, &t2)
	t3.MulByX(&t3)

	pt.Add(&pt, &t3)
	pt.Sub(&pt, &t1)
	pt.Sub(&pt, &t2)
	pt.Sub(&pt, a)
	return g2.Set(&pt)
}

// Neg negates this point
func (g2 *G2) Neg(a *G2) *G2 {
	g2.Set(a)
	g2.y.CNeg(&a.y, -(a.IsIdentity() - 1))
	return g2
}

// Set copies a into g2
func (g2 *G2) Set(a *G2) *G2 {
	g2.x.Set(&a.x)
	g2.y.Set(&a.y)
	g2.z.Set(&a.z)
	return g2
}

// BigInt returns the x and y as big.Ints in affine
func (g2 *G2) BigInt() (x, y *big.Int) {
	var t G2
	out := t.ToUncompressed()
	x = new(big.Int).SetBytes(out[:WideFieldBytes])
	y = new(big.Int).SetBytes(out[WideFieldBytes:])
	return
}

// SetBigInt creates a point from affine x, y
// and returns the point if it is on the curve
func (g2 *G2) SetBigInt(x, y *big.Int) (*G2, error) {
	var tt [DoubleWideFieldBytes]byte

	if len(x.Bytes()) == 0 && len(y.Bytes()) == 0 {
		return g2.Identity(), nil
	}
	x.FillBytes(tt[:WideFieldBytes])
	y.FillBytes(tt[WideFieldBytes:])

	return g2.FromUncompressed(&tt)
}

// ToCompressed serializes this element into compressed form.
func (g2 *G2) ToCompressed() [WideFieldBytes]byte {
	var out [WideFieldBytes]byte
	var t G2
	t.ToAffine(g2)
	xABytes := t.x.A.Bytes()
	xBBytes := t.x.B.Bytes()
	copy(out[:FieldBytes], internal.ReverseScalarBytes(xBBytes[:]))
	copy(out[FieldBytes:], internal.ReverseScalarBytes(xABytes[:]))
	isInfinity := byte(g2.IsIdentity())
	// Compressed flag
	out[0] |= 1 << 7
	// Is infinity
	out[0] |= (1 << 6) & -isInfinity
	// Sign of y only set if not infinity
	out[0] |= (byte(t.y.LexicographicallyLargest()) << 5) & (isInfinity - 1)
	return out
}

// FromCompressed deserializes this element from compressed form.
func (g2 *G2) FromCompressed(input *[WideFieldBytes]byte) (*G2, error) {
	var xFp, yFp fp2
	var xA, xB [FieldBytes]byte
	var p G2
	compressedFlag := int((input[0] >> 7) & 1)
	infinityFlag := int((input[0] >> 6) & 1)
	sortFlag := int((input[0] >> 5) & 1)

	if compressedFlag != 1 {
		return nil, errors.New("compressed flag must be set")
	}

	if infinityFlag == 1 {
		return g2.Identity(), nil
	}

	copy(xB[:], internal.ReverseScalarBytes(input[:FieldBytes]))
	copy(xA[:], internal.ReverseScalarBytes(input[FieldBytes:]))
	// Mask away the flag bits
	xB[FieldBytes-1] &= 0x1F
	_, validA := xFp.A.SetBytes(&xA)
	_, validB := xFp.B.SetBytes(&xB)

	if validA&validB != 1 {
		return nil, errors.New("invalid bytes - not in field")
	}

	// Recover a y-coordinate given x by y = sqrt(x^3 + 4)
	yFp.Square(&xFp)
	yFp.Mul(&yFp, &xFp)
	yFp.Add(&yFp, &curveG2B)

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
	return g2.Set(&p), nil
}

// ToUncompressed serializes this element into uncompressed form.
func (g2 *G2) ToUncompressed() [DoubleWideFieldBytes]byte {
	var out [DoubleWideFieldBytes]byte
	var t G2
	t.ToAffine(g2)
	bytes := t.x.B.Bytes()
	copy(out[:FieldBytes], internal.ReverseScalarBytes(bytes[:]))
	bytes = t.x.A.Bytes()
	copy(out[FieldBytes:WideFieldBytes], internal.ReverseScalarBytes(bytes[:]))
	bytes = t.y.B.Bytes()
	copy(out[WideFieldBytes:WideFieldBytes+FieldBytes], internal.ReverseScalarBytes(bytes[:]))
	bytes = t.y.A.Bytes()
	copy(out[WideFieldBytes+FieldBytes:], internal.ReverseScalarBytes(bytes[:]))
	isInfinity := byte(g2.IsIdentity())
	out[0] |= (1 << 6) & -isInfinity
	return out
}

// FromUncompressed deserializes this element from uncompressed form.
func (g2 *G2) FromUncompressed(input *[DoubleWideFieldBytes]byte) (*G2, error) {
	var a, b fp
	var t [FieldBytes]byte
	var p G2
	infinityFlag := int((input[0] >> 6) & 1)

	if infinityFlag == 1 {
		return g2.Identity(), nil
	}

	copy(t[:], internal.ReverseScalarBytes(input[:FieldBytes]))
	// Mask away top bits
	t[FieldBytes-1] &= 0x1F

	_, valid := b.SetBytes(&t)
	if valid == 0 {
		return nil, errors.New("invalid bytes - x.B not in field")
	}
	copy(t[:], internal.ReverseScalarBytes(input[FieldBytes:WideFieldBytes]))
	_, valid = a.SetBytes(&t)
	if valid == 0 {
		return nil, errors.New("invalid bytes - x.A not in field")
	}

	p.x.B.Set(&b)
	p.x.A.Set(&a)

	copy(t[:], internal.ReverseScalarBytes(input[WideFieldBytes:WideFieldBytes+FieldBytes]))
	_, valid = b.SetBytes(&t)
	if valid == 0 {
		return nil, errors.New("invalid bytes - y.B not in field")
	}
	copy(t[:], internal.ReverseScalarBytes(input[FieldBytes+WideFieldBytes:]))
	_, valid = a.SetBytes(&t)
	if valid == 0 {
		return nil, errors.New("invalid bytes - y.A not in field")
	}

	p.y.B.Set(&b)
	p.y.A.Set(&a)
	p.z.SetOne()

	if p.IsOnCurve() == 0 {
		return nil, errors.New("point is not on the curve")
	}
	if p.InCorrectSubgroup() == 0 {
		return nil, errors.New("point is not in correct subgroup")
	}
	return g2.Set(&p), nil
}

// ToAffine converts the point into affine coordinates
func (g2 *G2) ToAffine(a *G2) *G2 {
	var wasInverted int
	var zero, x, y, z fp2
	_, wasInverted = z.Invert(&a.z)
	x.Mul(&a.x, &z)
	y.Mul(&a.y, &z)

	g2.x.CMove(&zero, &x, wasInverted)
	g2.y.CMove(&zero, &y, wasInverted)
	g2.z.CMove(&zero, z.SetOne(), wasInverted)
	return g2
}

// GetX returns the affine X coordinate
func (g2 *G2) GetX() *fp2 {
	var t G2
	t.ToAffine(g2)
	return &t.x
}

// GetY returns the affine Y coordinate
func (g2 *G2) GetY() *fp2 {
	var t G2
	t.ToAffine(g2)
	return &t.y
}

// Equal returns 1 if the two points are equal 0 otherwise.
func (g2 *G2) Equal(rhs *G2) int {
	var x1, x2, y1, y2 fp2
	var e1, e2 int

	// This technique avoids inversions
	x1.Mul(&g2.x, &rhs.z)
	x2.Mul(&rhs.x, &g2.z)

	y1.Mul(&g2.y, &rhs.z)
	y2.Mul(&rhs.y, &g2.z)

	e1 = g2.z.IsZero()
	e2 = rhs.z.IsZero()

	// Both at infinity or coordinates are the same
	return (e1 & e2) | (^e1 & ^e2)&x1.Equal(&x2)&y1.Equal(&y2)
}

// CMove sets g2 = arg1 if choice == 0 and g2 = arg2 if choice == 1
func (g2 *G2) CMove(arg1, arg2 *G2, choice int) *G2 {
	g2.x.CMove(&arg1.x, &arg2.x, choice)
	g2.y.CMove(&arg1.y, &arg2.y, choice)
	g2.z.CMove(&arg1.z, &arg2.z, choice)
	return g2
}

// SumOfProducts computes the multi-exponentiation for the specified
// points and scalars and stores the result in `g2`.
// Returns an error if the lengths of the arguments is not equal.
func (g2 *G2) SumOfProducts(points []*G2, scalars []*native.Field) (*G2, error) {
	const Upper = 256
	const W = 4
	const Windows = Upper / W // careful--use ceiling division in case this doesn't divide evenly
	var sum G2
	if len(points) != len(scalars) {
		return nil, fmt.Errorf("length mismatch")
	}

	bucketSize := 1 << W
	windows := make([]G2, Windows)
	bytes := make([][32]byte, len(scalars))
	buckets := make([]G2, bucketSize)
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

	g2.Identity()
	for i := len(windows) - 1; i >= 0; i-- {
		for j := 0; j < W; j++ {
			g2.Double(g2)
		}

		g2.Add(g2, &windows[i])
	}
	return g2, nil
}

func (g2 *G2) psi(a *G2) *G2 {
	g2.x.FrobeniusMap(&a.x)
	g2.y.FrobeniusMap(&a.y)
	// z = frobenius(z)
	g2.z.FrobeniusMap(&a.z)

	// x = frobenius(x)/((u+1)^((p-1)/3))
	g2.x.Mul(&g2.x, &psiCoeffX)
	// y = frobenius(y)/(u+1)^((p-1)/2)
	g2.y.Mul(&g2.y, &psiCoeffY)

	return g2
}

func (g2 *G2) psi2(a *G2) *G2 {
	// x = frobenius^2(x)/2^((p-1)/3); note that q^2 is the order of the field.
	g2.x.Mul(&a.x, &psi2CoeffX)
	// y = -frobenius^2(y); note that q^2 is the order of the field.
	g2.y.Neg(&a.y)
	g2.z.Set(&a.z)
	return g2
}

func (g2 *G2) sswu(u *fp2) *G2 {
	/// simplified swu map for q = 9 mod 16 where AB == 0
	// <https://www.ietf.org/archive/id/draft-irtf-cfrg-hash-to-curve-11.html>
	var tv1, tv2, x1, x2, gx1, gx2, x, y, y2, t fp2

	tv1.Square(u)
	tv1.Mul(&tv1, &sswuMapZ)

	tv2.Square(&tv1)

	x1.Add(&tv1, &tv2)
	x1.Invert(&x1)
	x1.Add(&x1, (&fp2{}).SetOne())
	x1.CMove(&x1, &sswuMapZInv, x1.IsZero())
	x1.Mul(&x1, &swwuMapMbDivA)

	gx1.Square(&x1)
	gx1.Add(&gx1, &sswuMapA)
	gx1.Mul(&gx1, &x1)
	gx1.Add(&gx1, &sswuMapB)

	x2.Mul(&tv1, &x1)

	tv2.Mul(&tv2, &tv1)

	gx2.Mul(&gx1, &tv2)

	_, e2 := t.Sqrt(&gx1)

	x.CMove(&x2, &x1, e2)
	y2.CMove(&gx2, &gx1, e2)

	y.Sqrt(&y2)

	y.CNeg(&y, u.Sgn0()^y.Sgn0())
	g2.x.Set(&x)
	g2.y.Set(&y)
	g2.z.SetOne()
	return g2
}

func (g2 *G2) isogenyMap(a *G2) *G2 {
	const Degree = 4
	var xs [Degree]fp2
	xs[0].SetOne()
	xs[1].Set(&a.x)
	xs[2].Square(&a.x)
	for i := 3; i < Degree; i++ {
		xs[i].Mul(&xs[i-1], &a.x)
	}

	xNum := computeKFp2(xs[:], g2IsoXNum)
	xDen := computeKFp2(xs[:], g2IsoXDen)
	yNum := computeKFp2(xs[:], g2IsoYNum)
	yDen := computeKFp2(xs[:], g2IsoYDen)

	g2.x.Invert(&xDen)
	g2.x.Mul(&g2.x, &xNum)

	g2.y.Invert(&yDen)
	g2.y.Mul(&g2.y, &yNum)
	g2.y.Mul(&g2.y, &a.y)
	g2.z.SetOne()
	return g2
}

func computeKFp2(xxs []fp2, k []fp2) fp2 {
	var xx, t fp2
	for i := range k {
		xx.Add(&xx, t.Mul(&xxs[i], &k[i]))
	}
	return xx
}
