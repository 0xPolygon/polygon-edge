package bls

import (
	"crypto/sha256"
	"errors"
	"math/big"
)

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
	g1, err := HashToG1(message)
	if err != nil {
		return [2]*big.Int{}, err
	}

	return G1ToBigInt(g1), nil
}

// Converts message to G1 point
func HashToG1(message []byte) (*G1, error) {
	if MapToModeSolidity {
		hashRes, err := hashToFpXMDSHA256(message, GetDomain(), 2)
		if err != nil {
			return nil, err
		}

		p0, p1 := new(G1), new(G1)
		u0, u1 := hashRes[0], hashRes[1]

		if err := MapToG1(p0, u0); err != nil {
			return nil, err
		}

		if err := MapToG1(p1, u1); err != nil {
			return nil, err
		}

		G1Add(p0, p0, p1)
		G1Normalize(p0, p0)

		return p0, nil
	}

	g1 := new(G1)
	if err := g1.HashAndMapTo(message); err != nil {
		return nil, err
	}

	return g1, nil
}

// colects public keys from the BlsKeys
func collectPublicKeys(keys []*PrivateKey) []*PublicKey {
	pubKeys := make([]*PublicKey, len(keys))

	for i, key := range keys {
		pubKeys[i] = key.PublicKey()
	}

	return pubKeys
}

func hashToFpXMDSHA256(msg []byte, domain []byte, count int) ([]*Fp, error) {
	randBytes, err := expandMsgSHA256XMD(msg, domain, count*48)
	if err != nil {
		return nil, err
	}

	els := make([]*Fp, count)

	for i := 0; i < count; i++ {
		els[i], err = from48Bytes(randBytes[i*48 : (i+1)*48])
		if err != nil {
			return nil, err
		}
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

func fromBytes(in []byte) (*Fp, error) {
	if len(in) != 32 {
		return nil, errors.New("input string should be equal 32 bytes")
	}

	fe := BytesToFp(in)

	FpMul(&fe, &fe, &r2)

	return &fe, nil
}

func from48Bytes(in []byte) (*Fp, error) {
	if len(in) != 48 {
		return nil, errors.New("input string should be equal 48 bytes")
	}

	a0 := make([]byte, 32)
	a1 := make([]byte, 32)

	copy(a0[8:32], in[:24])
	copy(a1[8:32], in[24:])

	e0, err := fromBytes(a0)
	if err != nil {
		return nil, err
	}

	e1, err := fromBytes(a1)
	if err != nil {
		return nil, err
	}

	// F = 2 ^ 192 * R
	F := newFp(0xd9e291c2cdd22cd6,
		0xc722ccf2a40f0271,
		0xa49e35d611a2ac87,
		0x2e1043978c993ec8)

	FpMul(e0, e0, &F)
	FpAdd(e1, e1, e0)

	return e1, nil
}
