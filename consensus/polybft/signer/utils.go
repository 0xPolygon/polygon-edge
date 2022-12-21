package bls

import (
	"bytes"
	"crypto/rand"
	"math/big"

	ellipticcurve "github.com/consensys/gnark-crypto/ecc/bn254"
)

var (
	maxBigInt, _    = new(big.Int).SetString("30644e72e131a029b85045b68181585d2833e84879b9709143e1f593f0000001", 16)
	ellipticCurveG2 *ellipticcurve.G2Affine
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
	pg1, err := ellipticcurve.HashToG1(message, GetDomain())
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
