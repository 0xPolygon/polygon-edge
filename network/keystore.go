package network

import (
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/0xPolygon/polygon-sdk/secrets"
	"github.com/libp2p/go-libp2p-core/crypto"
)

var Libp2pKeyName = "libp2p.key"

// ReadLibp2pKey reads the libp2p private key from the passed in data directory.
//
// The key must be named 'libp2p.key'
//
// If no key is found, it is generated and returned
func ReadLibp2pKey(dataDir string) (crypto.PrivKey, error) {
	if dataDir == "" {
		// use an in-memory key
		priv, _, err := crypto.GenerateKeyPair(crypto.Secp256k1, 256)
		if err != nil {
			return nil, err
		}

		return priv, nil
	}

	path := filepath.Join(dataDir, Libp2pKeyName)
	_, err := os.Stat(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to stat (%s): %v", path, err)
	}

	if os.IsNotExist(err) {
		// The key doesn't exist, generate it
		priv, _, err := crypto.GenerateKeyPair(crypto.Secp256k1, 256)
		if err != nil {
			return nil, err
		}

		buf, err := crypto.MarshalPrivateKey(priv)
		if err != nil {
			return nil, err
		}

		if err := ioutil.WriteFile(
			path,
			[]byte(hex.EncodeToString(buf)),
			0600,
		); err != nil {
			return nil, err
		}

		return priv, nil
	}

	// exists
	raw, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	buf, err := hex.DecodeString(string(raw))
	if err != nil {
		return nil, err
	}

	key, err := crypto.UnmarshalPrivateKey(buf)
	if err != nil {
		return nil, err
	}

	return key, nil
}

func ReadLibp2pKeyNEW(manager secrets.SecretsManager) (crypto.PrivKey, error) {
	libp2pKey, err := manager.GetSecret(secrets.NetworkKey)
	if err != nil {
		return nil, err
	}

	return ParseLibp2pKey(libp2pKey.([]byte))
}

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
