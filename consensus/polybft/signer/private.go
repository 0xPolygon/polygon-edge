package bls

import (
	"math/big"

	"errors"

	bn256 "github.com/umbracle/go-eth-bn256"
)

type PrivateKey struct {
	p *big.Int
}

// PublicKey returns the public key from the PrivateKey
func (p *PrivateKey) PublicKey() *PublicKey {
	return &PublicKey{p: new(bn256.G2).ScalarBaseMult(p.p)}
}

// Sign generates a simple BLS signature of the given message
func (p *PrivateKey) Sign(message []byte) (*Signature, error) {
	hashPoint, err := g1HashToPoint(message)
	if err != nil {
		return &Signature{}, err
	}
	return &Signature{p: new(bn256.G1).ScalarMult(hashPoint, p.p)}, nil
}

// MarshalJSON marshal the key to bytes.
func (p *PrivateKey) MarshalJSON() ([]byte, error) {
	if p.p == nil {
		return nil, errors.New("cannot marshal empty private key")
	}
	return p.p.MarshalJSON()
}

// UnmarshalPrivateKey reads the private key from the given byte array
func UnmarshalPrivateKey(data []byte) (*PrivateKey, error) {
	p := new(big.Int)
	err := p.UnmarshalJSON(data)
	return &PrivateKey{p: p}, err
}
