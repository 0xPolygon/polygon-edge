package crypto

import (
	"crypto/ecdsa"

	"github.com/0xPolygon/polygon-edge/types"
)

type Key interface {
	Address() types.Address
	Sign(hash []byte) ([]byte, error)
}

var _ Key = (*ECDSAKey)(nil)

type ECDSAKey struct {
	priv *ecdsa.PrivateKey
	pub  *ecdsa.PublicKey
	addr types.Address
}

// NewECDSAKey returns new instance of ECDSAKey
func NewECDSAKey(priv *ecdsa.PrivateKey) *ECDSAKey {
	return &ECDSAKey{
		priv: priv,
		pub:  &priv.PublicKey,
		addr: PubKeyToAddress(&priv.PublicKey),
	}
}

// GenerateECDSAKey generates an ECDSA private key and wraps it into ECDSA key
// (that is a wrapper for ECDSA private key and holds private key, public key and address)
func GenerateECDSAKey() (*ECDSAKey, error) {
	privKey, err := GenerateECDSAPrivateKey()
	if err != nil {
		return nil, err
	}

	return NewECDSAKey(privKey), nil
}

// NewECDSAKeyFromRawPrivECDSA parses provided
func NewECDSAKeyFromRawPrivECDSA(rawPrivKey []byte) (*ECDSAKey, error) {
	priv, err := ParseECDSAPrivateKey(rawPrivKey)
	if err != nil {
		return nil, err
	}

	return NewECDSAKey(priv), nil
}

// Sign uses the private key and signs the provided hash which produces a signature as a result
func (k *ECDSAKey) Sign(hash []byte) ([]byte, error) {
	return Sign(k.priv, hash)
}

// Address is a getter function for Ethereum address derived from the public key
func (k *ECDSAKey) Address() types.Address {
	return k.addr
}

// MarshallPrivateKey returns 256-bit big-endian binary-encoded representation of the private key
// padded to a length of 32 bytes.
func (k *ECDSAKey) MarshallPrivateKey() ([]byte, error) {
	btcPrivKey, err := convertToBtcPrivKey(k.priv)
	if err != nil {
		return nil, err
	}

	return btcPrivKey.Serialize(), nil
}

// String returns hex encoded ECDSA address
func (k *ECDSAKey) String() string {
	return k.addr.String()
}
