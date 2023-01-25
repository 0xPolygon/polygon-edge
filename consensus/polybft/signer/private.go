package bls

import (
	"math/big"

	bn256 "github.com/umbracle/go-eth-bn256"
)

// PrivateKey holds private key for bls implementation
type PrivateKey struct {
	s *big.Int
}

// PublicKey returns the public key from the PrivateKey
func (p *PrivateKey) PublicKey() *PublicKey {
	g2 := new(bn256.G2)
	g2 = g2.ScalarMult(g2Point, p.s)

	return &PublicKey{g2: g2}
}

// MarshalJSON marshal the key to json bytes.
func (p *PrivateKey) MarshalJSON() ([]byte, error) {
	return p.s.MarshalJSON()
}

// Sign generates a simple BLS signature of the given message
func (p *PrivateKey) Sign(message []byte) (*Signature, error) {
	point, err := hashToPoint(message, domain)
	if err != nil {
		return nil, err
	}

	g1 := new(bn256.G1)
	g1 = g1.ScalarMult(point, p.s)

	return &Signature{g1: g1}, nil
}

// UnmarshalPrivateKey unmarshals the private key from the given byte array
func UnmarshalPrivateKey(data []byte) (*PrivateKey, error) {
	s := new(big.Int)

	if err := s.UnmarshalJSON(data); err != nil {
		return nil, err
	}

	return &PrivateKey{s: s}, nil
}
