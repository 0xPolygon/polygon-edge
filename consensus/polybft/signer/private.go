package bls

import (
	"errors"

	"github.com/prysmaticlabs/go-bls"
)

type PrivateKey struct {
	secret *bls.SecretKey
}

func NewPrivateKey() *PrivateKey {
	var secret *bls.SecretKey

	secret.SetByCSPRNG()

	return &PrivateKey{secret: secret}
}

// PublicKey returns the public key from the PrivateKey
func (p *PrivateKey) PublicKey() *PublicKey {
	return &PublicKey{p: p.secret.GetPublicKey()}
}

// Sign generates a simple BLS signature of the given message
func (p *PrivateKey) Sign(message []byte) *Signature {
	return &Signature{sig: p.secret.Sign(message)}
}

// MarshalJSON marshal the key to bytes.
func (p *PrivateKey) MarshalJSON() ([]byte, error) {
	if p.secret == nil {
		return nil, errors.New("cannot marshal empty private key")
	}

	return []byte(p.secret.SerializeToHexStr()), nil
}

// UnmarshalPrivateKey reads the private key from the given byte array
func UnmarshalPrivateKey(data []byte) (*PrivateKey, error) {
	var secret *bls.SecretKey
	if err := secret.DeserializeHexStr(string(data)); err != nil {
		return nil, err
	}

	return &PrivateKey{secret: secret}, nil
}
