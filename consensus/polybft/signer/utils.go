package bls

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"math/big"

	"github.com/consensys/gnark-crypto/ecc/bn254"
	"github.com/consensys/gnark-crypto/ecc/bn254/fp"
)

var (
	maxBigInt, _ = new(big.Int).SetString("30644e72e131a029b85045b68181585d2833e84879b9709143e1f593f0000001", 16)
	baseG2       *bn254.G2Affine

	// fpR1 = fp.Element{0xd35d438dc58f0d9d, 0x0a78eb28f5c70b3d, 0x666ea36f7879462c, 0x0e0a77c19a07df2f}
	fpR2 = fp.Element{0xf32cfc5b538afa89, 0xb5e71911d44501fb, 0x47ab1eff0a417ff6, 0x06d89f71cab8351f}

	// b coefficient for G1
	fpBCoef = fp.Element{0x7a17caa950ad28d7, 0x1f6ac17ae15521b9, 0x334bea4e696bd284, 0x2a1f6744ce179d8e}

	// sqrt(-3)
	fpSqrtMinus3 = fp.Element{0x3e424383c39ad5b9, 0x28ed3f00245fdc6a, 0x4a2e7e8c1e5ebdfa, 0x05b858f624573163}

	// (sqrt(-3) - 1) / 2
	fpZZ = fp.Element{0x71930c11d782e155, 0xa6bb947cffbe3323, 0xaa303344d4741444, 0x2c3b3f0d26594943}

	fpOne = fp.Element{0xd35d438dc58f0d9d, 0x0a78eb28f5c70b3d, 0x666ea36f7879462c, 0x0e0a77c19a07df2f}

	// -1
	fpNegativeOne = fp.Element{0x68c3488912edefaa, 0x8d087f6872aabf4f, 0x51e1a24709081231, 0x2259d6b14729c0fa}

	bigPMinus1Over2, _ = new(big.Int).SetString("183227397098d014dc2822db40c0ac2ecbc0b548b438e5469e10460b6c3e7ea3", 16)

	bigPPlus1Over4, _ = new(big.Int).SetString("c19139cb84c680a6e14116da060561765e05aa45a1c72a34f082305b61f3f52", 16)
)

func init() {
	g2 := bn254.G2Jac{}
	g2.X.A0 = fp.Element{0x8e83b5d102bc2026, 0xdceb1935497b0172, 0xfbb8264797811adf, 0x19573841af96503b}
	g2.X.A1 = fp.Element{0xafb4737da84c6140, 0x6043dd5a5802d8c4, 0x09e950fc52a02f86, 0x14fef0833aea7b6b}
	g2.Y.A0 = fp.Element{0x619dfa9d886be9f6, 0xfe7fd297f59e9b78, 0xff9e1a62231b7dfe, 0x28fd7eebae9e4206}
	g2.Y.A1 = fp.Element{0x64095b56c71856ee, 0xdc57f922327d3cbb, 0x55f935be33351076, 0x0da4a0e693fd6482}
	g2.Z.A0 = fp.Element{0xd35d438dc58f0d9d, 0x0a78eb28f5c70b3d, 0x666ea36f7879462c, 0x0e0a77c19a07df2f}
	g2.Z.A1 = fp.Element{0x0000000000000000, 0x0000000000000000, 0x0000000000000000, 0x0000000000000000}

	baseG2 = new(bn254.G2Affine).FromJacobian(&g2)
}

// GenerateBlsKey creates a random private and its corresponding public keys
func GenerateBlsKey() (*PrivateKey, error) {
	s, err := rand.Int(rand.Reader, maxBigInt)
	if err != nil {
		return nil, err
	}

	p := new(big.Int)
	p.SetBytes(s.Bytes())

	return &PrivateKey{p: p}, nil
}

// CreateRandomBlsKeys creates an array of random private and their corresponding public keys
func CreateRandomBlsKeys(total int) ([]*PrivateKey, error) {
	blsKeys := make([]*PrivateKey, total)

	for i := 0; i < total; i++ {
		blsKey, err := GenerateBlsKey()
		if err != nil {
			return nil, err
		}

		blsKeys[i] = blsKey
	}

	return blsKeys, nil
}

// MarshalMessageToBigInt marshalls message into two big ints
// first we must convert message bytes to point and than for each coordinate we create big int
func MarshalMessageToBigInt(message []byte) ([2]*big.Int, error) {
	pg1, err := HashToG107(message)
	if err != nil {
		return [2]*big.Int{}, err
	}

	var b bytes.Buffer

	if err := bn254.NewEncoder(&b, bn254.RawEncoding()).Encode(pg1); err != nil {
		return [2]*big.Int{}, err
	}

	buf := b.Bytes()

	return [2]*big.Int{
		new(big.Int).SetBytes(buf[0:32]),
		new(big.Int).SetBytes(buf[32:64]),
	}, nil
}

// HashToG107 converts message to G1 point https://datatracker.ietf.org/doc/html/draft-irtf-cfrg-hash-to-curve-07
func HashToG107(message []byte) (*bn254.G1Affine, error) {
	hashRes, err := hashToFpXMDSHA256(message, GetDomain(), 2)
	if err != nil {
		return nil, err
	}

	p0, err := mapToPointFT(&hashRes[0])
	if err != nil {
		return nil, err
	}

	p1, err := mapToPointFT(&hashRes[1])
	if err != nil {
		return nil, err
	}

	fmt.Println(p0.String())
	fmt.Println(p1.String())

	p0.Add(p0, p1)

	fmt.Println(p0.String())

	return p0, nil
}

func hashToFpXMDSHA256(msg []byte, domain []byte, count int) ([]fp.Element, error) {
	randBytes, err := expandMsgSHA256XMD(msg, domain, count*48)
	if err != nil {
		return nil, err
	}

	els := make([]fp.Element, count)

	for i := 0; i < count; i++ {
		value, err := from48Bytes(randBytes[i*48 : (i+1)*48])
		if err != nil {
			return nil, err
		}

		els[i] = *value
	}

	return els, nil
}

func expandMsgSHA256XMD(msg []byte, domain []byte, outLen int) ([]byte, error) {
	h := sha256.New()

	if len(domain) > 255 {
		return nil, errors.New("invalid domain length")
	}

	domainLen := uint8(len(domain))
	// DST_prime = DST || I2OSP(len(DST), 1)
	// b_0 = H(Z_pad || msg || l_i_b_str || I2OSP(0, 1) || DST_prime)
	_, _ = h.Write(make([]byte, h.BlockSize()))
	_, _ = h.Write(msg)
	_, _ = h.Write([]byte{uint8(outLen >> 8), uint8(outLen)})
	_, _ = h.Write([]byte{0})
	_, _ = h.Write(domain)
	_, _ = h.Write([]byte{domainLen})
	b0 := h.Sum(nil)

	// b_1 = H(b_0 || I2OSP(1, 1) || DST_prime)
	h.Reset()
	_, _ = h.Write(b0)
	_, _ = h.Write([]byte{1})
	_, _ = h.Write(domain)
	_, _ = h.Write([]byte{domainLen})
	b1 := h.Sum(nil)

	// b_i = H(strxor(b_0, b_(i - 1)) || I2OSP(i, 1) || DST_prime)
	ell := (outLen + h.Size() - 1) / h.Size()
	bi := b1
	out := make([]byte, outLen)

	for i := 1; i < ell; i++ {
		h.Reset()
		// b_i = H(strxor(b_0, b_(i - 1)) || I2OSP(i, 1) || DST_prime)
		tmp := make([]byte, h.Size())
		for j := 0; j < h.Size(); j++ {
			tmp[j] = b0[j] ^ bi[j]
		}

		_, _ = h.Write(tmp)
		_, _ = h.Write([]byte{1 + uint8(i)})
		_, _ = h.Write(domain)
		_, _ = h.Write([]byte{domainLen})

		// b_1 || ... || b_(ell - 1)
		copy(out[(i-1)*h.Size():i*h.Size()], bi[:])
		bi = h.Sum(nil)
	}

	// b_ell
	copy(out[(ell-1)*h.Size():], bi[:])

	return out[:outLen], nil
}

// MapToPointTI applies Fouque Tibouchi map to point method
func mapToPointFT(t *fp.Element) (*bn254.G1Affine, error) {
	g1J := bn254.G1Jac{}

	a, t2, w := fp.Element{}, fp.Element{}, fp.Element{}

	// t^2 + fpBCoef + fpOne // 	a.Add(w.Add(t2.Square(t), &fpBCoef), &fpOne)
	t2.Square(t)
	a.Add(&t2, &fpBCoef)
	a.Add(&a, &fpOne)

	st := fp.Element{}
	st.Mul(&fpSqrtMinus3, t)

	w0 := fp.Element{}
	w0.Mul(&st, &a)
	w0.Inverse(&w0)

	w.Square(&st)
	w.Mul(&w, &w0)

	e := fp.Element{}
	isQuadraticNonResidue := !e.Exp(*t, bigPMinus1Over2).IsOne()

	// x1 = fpZZ - t * w
	tw, x1 := fp.Element{}, fp.Element{}
	x1.Sub(&fpZZ, tw.Mul(t, &w))

	y := fp.Element{}
	y.Square(&x1)
	y.Mul(&y, &x1)
	y.Add(&y, &fpBCoef)

	if customSqrt(&y, &y) {
		if isQuadraticNonResidue {
			y.Neg(&y)
		}

		g1J.X = x1
		g1J.Y = y
		g1J.Z = fpOne

		return new(bn254.G1Affine).FromJacobian(&g1J), nil
	}

	// x2
	x2 := fp.Element{}
	x2.Sub(&fpNegativeOne, &x1)

	y.Square(&x2)
	y.Mul(&y, &x2)
	y.Add(&y, &fpBCoef)

	if customSqrt(&y, &y) {
		if isQuadraticNonResidue {
			y.Neg(&y)
		}

		g1J.X = x2
		g1J.Y = y
		g1J.Z = fpOne

		return new(bn254.G1Affine).FromJacobian(&g1J), nil
	}

	// x3
	x3 := fp.Element{}
	x3.Square(&a)
	x3.Square(&x3)
	x3.Mul(&x3, &w0)
	x3.Mul(&x3, &w0)
	x3.Add(&x3, &fpOne)

	y.Square(&x3)
	y.Mul(&y, &x3)
	y.Add(&y, &fpBCoef)

	if !customSqrt(&y, &y) {
		return nil, errors.New("bad implementation")
	}

	if isQuadraticNonResidue {
		y.Neg(&y)
	}

	g1J.X = x3
	g1J.Y = y
	g1J.Z = fpOne

	return new(bn254.G1Affine).FromJacobian(&g1J), nil
}

func customSqrt(c, a *fp.Element) bool {
	// if sqrt(y, y) {
	// func sqrt(c, a *fe) bool {
	// 	u, v := new(fe).set(a), new(fe)
	// 	exp(c, a, pPlus1Over4)
	// 	square(v, c)
	// 	return u.equal(v)
	// }
	u, v := fp.Element{}, fp.Element{}

	u.Set(a)
	c.Exp(*a, bigPPlus1Over4)
	v.Square(c)

	return u.Equal(&v)
}

func from48Bytes(in []byte) (*fp.Element, error) {
	if len(in) != 48 {
		return nil, errors.New("input string should be equal 48 bytes")
	}

	a0 := make([]byte, 32)
	copy(a0[8:32], in[:24])

	a1 := make([]byte, 32)
	copy(a1[8:32], in[24:])

	e0, err := fpFromBytes(a0)
	if err != nil {
		return nil, err
	}

	e1, err := fpFromBytes(a1)
	if err != nil {
		return nil, err
	}

	// F = 2 ^ 192 * R
	F := fp.Element{
		0xd9e291c2cdd22cd6,
		0xc722ccf2a40f0271,
		0xa49e35d611a2ac87,
		0x2e1043978c993ec8,
	}

	return e1.Add(e1, e0.Mul(e0, &F)), nil
}

func fpFromBytes(in []byte) (*fp.Element, error) {
	const size = 32

	if len(in) != size {
		return nil, errors.New("input string should be equal 32 bytes")
	}

	l := len(in)
	if l >= size {
		l = size
	}

	padded := make([]byte, size)

	copy(padded[size-l:], in[:])

	fe := fp.Element{}

	for i := 0; i < 4; i++ {
		a := size - i*8
		fe[i] = uint64(padded[a-1]) | uint64(padded[a-2])<<8 |
			uint64(padded[a-3])<<16 | uint64(padded[a-4])<<24 |
			uint64(padded[a-5])<<32 | uint64(padded[a-6])<<40 |
			uint64(padded[a-7])<<48 | uint64(padded[a-8])<<56
	}

	return fe.Mul(&fe, &fpR2), nil
}
