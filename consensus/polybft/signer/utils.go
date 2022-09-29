package bls

import (
	"crypto/rand"

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
