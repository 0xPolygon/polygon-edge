package signer

import (
	"crypto/ecdsa"

	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/types"
)

type ECDSASigner struct {
	key *ecdsa.PrivateKey
}

func NewECDSASigner(key *ecdsa.PrivateKey) *ECDSASigner {
	return &ECDSASigner{key: key}
}

func (s *ECDSASigner) Sign(message []byte) ([]byte, error) {
	return crypto.Sign(s.key, message)
}

func (s *ECDSASigner) Address() types.Address {
	return crypto.PubKeyToAddress(&s.key.PublicKey)
}

func (s *ECDSASigner) RecoverAddress(digest, signature []byte) (types.Address, error) {
	pub, err := crypto.SigToPub(digest, signature)
	if err != nil {
		return types.Address{}, err
	}

	return crypto.PubKeyToAddress(pub), nil
}
