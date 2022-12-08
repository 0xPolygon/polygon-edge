package bls

import (
	"errors"
	"math/big"

	"github.com/kilic/bn254"
)

type PrivateKey struct {
	p *big.Int
}

// PublicKey returns the public key from the PrivateKey
func (p *PrivateKey) PublicKey() *PublicKey {
	g2 := bn254.NewG2()
	public := g2.New()

	g2.MulScalar(public, g2.One(), p.p)

	return &PublicKey{p: public}
}

// Sign generates a simple BLS signature of the given message
func (p *PrivateKey) Sign(message []byte) (*Signature, error) {
	g := bn254.NewG1()

	signature, err := g.HashToCurveFT(message, GetDomain())
	if err != nil {
		return nil, err
	}

	g.MulScalar(signature, signature, p.p)

	return &Signature{signature}, nil
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
