package bls

import (
	"crypto/rand"
	"math/big"

	bn256 "github.com/umbracle/go-eth-bn256"
)

// GenerateBlsKey creates a random private and its corresponding public keys
func GenerateBlsKey() (*PrivateKey, error) {
	priv, _, err := bn256.RandomG2(rand.Reader)
	if err != nil {
		return nil, err
	}

	return &PrivateKey{
		p: priv,
	}, nil
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
	hashPoint, err := g1HashToPoint(message)
	if err != nil {
		return [2]*big.Int{}, err
	}

	buf := hashPoint.Marshal()
	res := [2]*big.Int{
		new(big.Int).SetBytes(buf[0:32]),
		new(big.Int).SetBytes(buf[32:64]),
	}

	return res, nil
}
