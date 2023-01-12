package bls

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"math/big"

	ellipticcurve "github.com/consensys/gnark-crypto/ecc/bn254"
	ellipticcurvefp "github.com/consensys/gnark-crypto/ecc/bn254/fp"
)

var (
	maxBigInt, _    = new(big.Int).SetString("30644e72e131a029b85045b68181585d2833e84879b9709143e1f593f0000001", 16)
	ellipticCurveG2 *ellipticcurve.G2Affine

	r1 = ellipticcurvefp.Element{0xd35d438dc58f0d9d, 0x0a78eb28f5c70b3d, 0x666ea36f7879462c, 0x0e0a77c19a07df2f}
	r2 = ellipticcurvefp.Element{0xf32cfc5b538afa89, 0xb5e71911d44501fb, 0x47ab1eff0a417ff6, 0x06d89f71cab8351f}
)

func init() {
	v1, _ := new(big.Int).SetString("10857046999023057135944570762232829481370756359578518086990519993285655852781", 10)
	v2, _ := new(big.Int).SetString("11559732032986387107991004021392285783925812861821192530917403151452391805634", 10)
	v3, _ := new(big.Int).SetString("8495653923123431417604973247489272438418190587263600148770280649306958101930", 10)
	v4, _ := new(big.Int).SetString("4082367875863433681332203403145435568316851327593401208105741076214120093531", 10)
	pk, _ := UnmarshalPublicKeyFromBigInt([4]*big.Int{v1, v2, v3, v4})
	ellipticCurveG2 = pk.p
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

	if err := ellipticcurve.NewEncoder(&b, ellipticcurve.RawEncoding()).Encode(pg1); err != nil {
		return [2]*big.Int{}, err
	}

	buf := b.Bytes()

	return [2]*big.Int{
		new(big.Int).SetBytes(buf[0:32]),
		new(big.Int).SetBytes(buf[32:64]),
	}, nil
}

// HashToG107 converts message to G1 point https://datatracker.ietf.org/doc/html/draft-irtf-cfrg-hash-to-curve-07
func HashToG107(message []byte) (*ellipticcurve.G1Affine, error) {
	hashRes, err := hashToFpXMDSHA256(message, GetDomain(), 2)
	if err != nil {
		return nil, err
	}

	p0 := ellipticcurve.MapToG1(hashRes[0])
	p1 := ellipticcurve.MapToG1(hashRes[1])

	return p0.Add(&p0, &p1), nil
}

func hashToFpXMDSHA256(msg []byte, domain []byte, count int) ([]ellipticcurvefp.Element, error) {
	randBytes, err := expandMsgSHA256XMD(msg, domain, count*48)
	if err != nil {
		return nil, err
	}

	els := make([]ellipticcurvefp.Element, count)

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

func from48Bytes(in []byte) (*ellipticcurvefp.Element, error) {
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
	F := ellipticcurvefp.Element{
		0xd9e291c2cdd22cd6,
		0xc722ccf2a40f0271,
		0xa49e35d611a2ac87,
		0x2e1043978c993ec8,
	}

	e0 = e0.Mul(e0, &F)
	e1 = e1.Mul(e1, e0)

	return e1, nil
}

func fpFromBytes(in []byte) (*ellipticcurvefp.Element, error) {
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

	component := [4]uint64{}

	for i := 0; i < 4; i++ {
		a := size - i*8
		component[i] = uint64(padded[a-1]) | uint64(padded[a-2])<<8 |
			uint64(padded[a-3])<<16 | uint64(padded[a-4])<<24 |
			uint64(padded[a-5])<<32 | uint64(padded[a-6])<<40 |
			uint64(padded[a-7])<<48 | uint64(padded[a-8])<<56
	}

	fe := ellipticcurvefp.Element{component[0], component[1], component[2], component[3]}

	return fe.Mul(&fe, &r2), nil
}
