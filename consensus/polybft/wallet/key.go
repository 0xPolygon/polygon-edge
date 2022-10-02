package wallet

import (
	"github.com/0xPolygon/pbft-consensus"
	"github.com/umbracle/ethgo"
)

type key struct {
	raw *Account
}

func newKey(raw *Account) *key {
	return &key{
		raw: raw,
	}
}

func (k *key) String() string {
	return k.raw.Ecdsa.Address().String()
}

func (k *key) Address() ethgo.Address {
	return k.raw.Ecdsa.Address()
}

func (k *key) NodeID() pbft.NodeID {
	return pbft.NodeID(k.String())
}

func (k *key) Sign(b []byte) ([]byte, error) {
	s, err := k.raw.Bls.Sign(b)
	if err != nil {
		return nil, err
	}
	return s.Marshal()
}
