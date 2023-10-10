package bls

import (
	"math/big"
	"strings"

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

// Marshal marshals private key hex (without 0x prefix) represented as a byte slice
func (p *PrivateKey) Marshal() ([]byte, error) {
	return []byte(strings.TrimPrefix(hex.EncodeBig(p.s), "0x")), nil
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
	s, err := hex.DecodeHexToBig(string(data))
	if err != nil {
		return nil, err
	}

	return &PrivateKey{s: s}, nil
}
