package bls

import (
	"math/big"

	bn256 "github.com/umbracle/go-eth-bn256"

	"github.com/0xPolygon/polygon-edge/helper/hex"
)

// PrivateKey holds private key for bls implementation
type PrivateKey struct {
	s *big.Int
}

// PublicKey returns the public key from the PrivateKey
func (p *PrivateKey) PublicKey() *PublicKey {
	g2 := new(bn256.G2).ScalarMult(g2Point, p.s)

	return &PublicKey{g2: g2}
}

// Marshal marshals private key to byte slice
func (p *PrivateKey) Marshal() ([]byte, error) {
	return p.s.MarshalText()
}

// Sign generates a simple BLS signature of the given message
func (p *PrivateKey) Sign(message, domain []byte) (*Signature, error) {
	point, err := hashToPoint(message, domain)
	if err != nil {
		return nil, err
	}

	g1 := new(bn256.G1).ScalarMult(point, p.s)

	return &Signature{g1: g1}, nil
}

// UnmarshalPrivateKey unmarshals the private key from the given byte slice
func UnmarshalPrivateKey(data []byte) (*PrivateKey, error) {
	s := new(big.Int)

	var err error

	// The given data might be a hex string either with 0x or without 0x prefix.
	// Both cases must be handled properly.
	if hex.IsHex(string(data)) {
		s, err = hex.DecodeHexToBig(string(data))
	} else {
		err = s.UnmarshalText(data)
	}

	if err != nil {
		return nil, err
	}

	return &PrivateKey{s: s}, nil
}
