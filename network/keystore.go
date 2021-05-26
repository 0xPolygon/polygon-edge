package network

import (
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

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
