package network

import (
	"encoding/hex"

	"github.com/0xPolygon/polygon-edge/secrets"
	"github.com/libp2p/go-libp2p/core/crypto"
)

// ReadLibp2pKey reads the private networking key from the secrets manager
func ReadLibp2pKey(manager secrets.SecretsManager) (crypto.PrivKey, error) {
	libp2pKey, err := manager.GetSecret(secrets.NetworkKey)
	if err != nil {
		return nil, err
	}

	return ParseLibp2pKey(libp2pKey)
}

// GenerateAndEncodeLibp2pKey generates a new networking private key, and encodes it into hex
func GenerateAndEncodeLibp2pKey() (crypto.PrivKey, []byte, error) {
	priv, _, err := crypto.GenerateKeyPair(crypto.Secp256k1, 256)
	if err != nil {
		return nil, nil, err
	}

	buf, err := crypto.MarshalPrivateKey(priv)
	if err != nil {
		return nil, nil, err
	}

	return priv, []byte(hex.EncodeToString(buf)), nil
}

// ParseLibp2pKey converts a byte array to a private key
func ParseLibp2pKey(key []byte) (crypto.PrivKey, error) {
	buf, err := hex.DecodeString(string(key))
	if err != nil {
		return nil, err
	}

	libp2pKey, err := crypto.UnmarshalPrivateKey(buf)
	if err != nil {
		return nil, err
	}

	return libp2pKey, nil
}
