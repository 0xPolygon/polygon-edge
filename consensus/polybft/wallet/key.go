package wallet

import (
	"fmt"

	ibftProto "github.com/0xPolygon/go-ibft/messages/proto"
	"github.com/umbracle/ethgo"
	protobuf "google.golang.org/protobuf/proto"

	bls "github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/types"
)

type Key struct {
	raw *Account
}

func NewKey(raw *Account) *Key {
	return &Key{
		raw: raw,
	}
}

// String returns hex encoded ECDSA address
func (k *Key) String() string {
	return k.raw.Ecdsa.Address().String()
}

// Address returns ECDSA address
func (k *Key) Address() ethgo.Address {
	return k.raw.Ecdsa.Address()
}

// Sign signs the provided digest with BLS key
// Used only to sign transactions
func (k *Key) Sign(digest []byte) ([]byte, error) {
	return k.SignWithDomain(digest, bls.DomainCommonSigning)
}

// SignWithDomain signs the provided digest with BLS key and provided domain
func (k *Key) SignWithDomain(digest, domain []byte) ([]byte, error) {
	signature, err := k.raw.Bls.Sign(digest, domain)
	if err != nil {
		return nil, err
	}

	return signature.Marshal()
}

// SignIBFTMessage signs the IBFT consensus message with ECDSA key
func (k *Key) SignIBFTMessage(msg *ibftProto.Message) (*ibftProto.Message, error) {
	msgRaw, err := protobuf.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("cannot marshal message: %w", err)
	}

	if msg.Signature, err = k.raw.Ecdsa.Sign(crypto.Keccak256(msgRaw)); err != nil {
		return nil, fmt.Errorf("cannot create message signature: %w", err)
	}

	return msg, nil
}

// RecoverSignerFromIBFTMessage recovers signer address from provided IBFT message based
// on signed message and its signature
func RecoverSignerFromIBFTMessage(msg *ibftProto.Message) (types.Address, error) {
	msgNoSig, err := msg.PayloadNoSig()
	if err != nil {
		return types.ZeroAddress, err
	}

	signerAddress, err := RecoverAddressFromSignature(msg.Signature, msgNoSig)
	if err != nil {
		return types.ZeroAddress, fmt.Errorf("failed to recover address from signature: %w", err)
	}

	return signerAddress, nil
}

// RecoverAddressFromSignature calculates keccak256 hash of provided rawContent
// and recovers signer address from given signature and hash
func RecoverAddressFromSignature(sig, rawContent []byte) (types.Address, error) {
	pub, err := crypto.RecoverPubkey(sig, crypto.Keccak256(rawContent))
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
