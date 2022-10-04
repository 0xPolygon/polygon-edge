package bls

import (
	"crypto/subtle"
	"fmt"
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

func UnmarshalPrivateKeyBinary(data []byte) (*PrivateKey, error) {
	if len(data) != 64 {
		return nil, fmt.Errorf("secret key must be %d bytes", 64)
	}

	zeros := make([]byte, len(data))
	if subtle.ConstantTimeCompare(data, zeros) == 1 {
		return nil, errors.New("secret key cannot be zero")
	}

	p := new(big.Int)
	outBytes := make([]byte, len(data))

	for i, j := 0, len(data)-1; j >= 0; i, j = i+1, j-1 {
		outBytes[i] = data[j]
	}

	p.SetBytes(outBytes)

	return &PrivateKey{p: p}, nil
}
