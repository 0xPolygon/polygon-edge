package bls

import (
	"crypto/rand"
	"encoding/hex"
	"io"
	"log"
	"math/big"

	bn256 "github.com/umbracle/go-eth-bn256"
)

var (
	// negated g2 point
	negG2Point = mustG2Point("198e9393920d483a7260bfb731fb5d25f1aa493335a9e71297e485b7aef312c21800deef121f1e76426a00665e5c4479674322d4f75edadd46debd5cd992f6ed275dc4a288d1afb3cbb1ac09187524c7db36395df7be3b99e673b13a075a65ec1d9befcd05a5323e6da4d435f3b617cdb3af83285c2df711ef39c01571827f9d") //nolint

	// g2 point
	g2Point = mustG2Point("198e9393920d483a7260bfb731fb5d25f1aa493335a9e71297e485b7aef312c21800deef121f1e76426a00665e5c4479674322d4f75edadd46debd5cd992f6ed090689d0585ff075ec9e99ad690c3395bc4b313370b38ef355acdadcd122975b12c85ea5db8c6deb4aab71808dcb408fe3d1e7690c43d37b4ce6cc0166fa7daa") //nolint
)

// GenerateBlsKey creates a random private key (and its corresponding public key)
func GenerateBlsKey() (*PrivateKey, error) {
	s, err := randomK(rand.Reader)
	if err != nil {
		return nil, err
	}

	return &PrivateKey{s: s}, nil
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

func mustG2Point(str string) *bn256.G2 {
	buf, err := hex.DecodeString(str)
	if err != nil {
		log.Fatal(err)
	}

	b := new(bn256.G2)

	if _, err := b.Unmarshal(buf); err != nil {
		log.Fatal(err)
	}

	return b
}

func randomK(r io.Reader) (k *big.Int, err error) {
	for {
		k, err = rand.Int(r, bn256.Order)
		if k.Sign() > 0 || err != nil {
			// The key cannot ever be zero, otherwise the cryptographic properties
			// of the curve do not hold.
			return
		}
	}
}
