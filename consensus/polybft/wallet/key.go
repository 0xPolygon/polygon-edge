package wallet

import (
	"github.com/umbracle/ethgo"
)

type Key struct {
	raw *Account
}

func NewKey(raw *Account) *Key {
	return &Key{
		raw: raw,
	}
}

func (k *Key) String() string {
	return k.raw.Ecdsa.Address().String()
}

func (k *Key) Address() ethgo.Address {
	return k.raw.Ecdsa.Address()
}

func (k *Key) NodeID() string {
	return k.String()
}

func (k *Key) Sign(b []byte) ([]byte, error) {
	s, err := k.raw.Bls.Sign(b)
	if err != nil {
		return nil, err
	}

	return s.Marshal()
}
