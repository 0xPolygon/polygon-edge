package wallet

import (
	"fmt"

	"github.com/0xPolygon/go-ibft/messages/proto"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/ethgo"
	protobuf "google.golang.org/protobuf/proto"
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

func (k *Key) Sign(b []byte) ([]byte, error) {
	s, err := k.raw.Bls.Sign(b)
	if err != nil {
		return nil, err
	}

	return s.Marshal()
}

// SignEcdsaMessage signs the proto message with ecdsa
func (k *Key) SignEcdsaMessage(msg *proto.Message) (*proto.Message, error) {
	raw, err := protobuf.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("cannot marshal message: %w", err)
	}

	if msg.Signature, err = k.raw.Ecdsa.Sign(raw); err != nil {
		return nil, fmt.Errorf("cannot create message signature: %w", err)
	}

	return msg, nil
}

// RecoverAddressFromSignature recovers signer address from the given digest and signature
func RecoverAddressFromSignature(sig, msg []byte) (types.Address, error) {
	pub, err := crypto.RecoverPubkey(sig, msg)
	if err != nil {
		return types.Address{}, fmt.Errorf("cannot recover address from signature: %w", err)
	}

	return crypto.PubKeyToAddress(pub), nil
}

// ECDSASigner implements ethgo.Key interface and it is used for signing using provided ECDSA key
type ECDSASigner struct {
	*Key
}

func NewEcdsaSigner(ecdsaKey *Key) *ECDSASigner {
	return &ECDSASigner{Key: ecdsaKey}
}

func (k *ECDSASigner) Sign(b []byte) ([]byte, error) {
	return k.raw.Ecdsa.Sign(b)
}
