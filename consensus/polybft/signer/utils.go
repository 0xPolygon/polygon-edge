package bls

import (
	"crypto/rand"
	"math/big"

	"github.com/kilic/bn254"
)

// GenerateBlsKey creates a random private and its corresponding public keys
func GenerateBlsKey() (*PrivateKey, error) {
	s, err := rand.Int(rand.Reader, bn254.Order)
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
	g1 := bn254.NewG1()

	pg1, err := g1.HashToCurveFT(message, GetDomain())
	if err != nil {
		return [2]*big.Int{}, err
	}

	buf := g1.ToBytes(pg1)
	res := [2]*big.Int{
		new(big.Int).SetBytes(buf[0:32]),
		new(big.Int).SetBytes(buf[32:64]),
	}

	return res, nil
}
