package bls

import (
	"errors"
	"math/big"

	ellipticcurve "github.com/consensys/gnark-crypto/ecc/bn254"
)

type PrivateKey struct {
	p *big.Int
}

// PublicKey returns the public key from the PrivateKey
func (p *PrivateKey) PublicKey() *PublicKey {
	public := new(ellipticcurve.G2Affine)
	public.Set(ellipticCurveG2)
	// g2.MulScalar(public, g2.One(), p.p)

	return &PublicKey{p: public.ScalarMultiplication(public, p.p)}
}

// Sign generates a signature of the given message
func (p *PrivateKey) Sign(message []byte) (*Signature, error) {
	/*
		g := bn254.NewG1()
		signature, err := g.HashToCurveFT(message, GetDomain())
		if err != nil {
			return nil, err
		}
		g.MulScalar(signature, signature, p.p)
	*/
	messagePoint, err := HashToG107(message)
	if err != nil {
		return nil, err
	}

	return &Signature{
		p: messagePoint.ScalarMultiplication(messagePoint, p.p),
	}, nil
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
