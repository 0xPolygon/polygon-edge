package keystore

import (
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/ethereum/go-ethereum/crypto"
)

// LocalKeystore loads the key from a local file
type LocalKeystore struct {
	Path string
}

// NewLocalKeystore creates a new local key store
func NewLocalKeystore(path string) *LocalKeystore {
	return &LocalKeystore{filepath.Join(path, "key")}
}

// Put implements the Keystore interface
func (k *LocalKeystore) Put(key *ecdsa.PrivateKey) error {
	err := ioutil.WriteFile(k.Path, []byte(hex.EncodeToString(crypto.FromECDSA(key))), 0600)
	return err
}

// Get implements the keystore interface
func (k *LocalKeystore) Get() (*ecdsa.PrivateKey, bool, error) {
	_, err := os.Stat(k.Path)
	if err != nil && !os.IsNotExist(err) {
		return nil, false, fmt.Errorf("Failed to stat (%s): %v", k.Path, err)
	}
	if os.IsNotExist(err) {
		return nil, false, nil
	}

	// exists
	data, err := ioutil.ReadFile(k.Path)
	if err != nil {
		return nil, false, err
	}
	keyStr, err := hex.DecodeString(string(data))
	if err != nil {
		return nil, false, err
	}
	key, err := crypto.ToECDSA(keyStr)
	if err != nil {
		return nil, false, err
	}
	return key, true, nil
}
