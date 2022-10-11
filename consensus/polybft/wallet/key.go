package wallet

import (
	"github.com/0xPolygon/pbft-consensus"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/types"
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

func (k *Key) NodeID() pbft.NodeID {
	return pbft.NodeID(k.String())
}

func (k *Key) Sign(b []byte) ([]byte, error) {
	s, err := k.raw.Bls.Sign(b)
	if err != nil {
		return nil, err
	}

	return s.Marshal()
}

func (k *Key) SignTx(chainID uint64, txn *types.Transaction) (*types.Transaction, error) {
	privKey, err := k.raw.GetEcdsaPrivateKey()
	if err != nil {
		return nil, err
	}

	signed, err := crypto.NewEIP155Signer(chainID).SignTx(txn, privKey)
	if err != nil {
		return nil, err
	}

	return signed, nil
}
