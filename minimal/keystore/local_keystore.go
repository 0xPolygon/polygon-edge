package keystore

import (
	"crypto/ecdsa"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/0xPolygon/minimal/crypto"
	"github.com/0xPolygon/minimal/helper/hex"
)

// LocalKeystore loads the key from a local file
type LocalKeystore struct {
	Path string
}

// NewLocalKeystore creates a new local key store
func NewLocalKeystore(path string) *LocalKeystore {
	return &LocalKeystore{filepath.Join(path, "key")}
}

// Get implements the keystore interface
func (k *LocalKeystore) Get() (*ecdsa.PrivateKey, error) {
	_, err := os.Stat(k.Path)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("Failed to stat (%s): %v", k.Path, err)
	}
	if os.IsNotExist(err) {
		key, err := crypto.GenerateKey()
		if err != nil {
			return nil, err
		}
		buf, err := crypto.MarshalPrivateKey(key)
		if err != nil {
			return nil, err
		}
		if err := ioutil.WriteFile(k.Path, []byte(hex.EncodeToString(buf)), 0600); err != nil {
			return nil, err
		}
		return key, nil
	}

	// exists
	buf, err := ioutil.ReadFile(k.Path)
	if err != nil {
		return nil, err
	}
	key, err := crypto.ParsePrivateKey(buf)
	if err != nil {
		return nil, err
	}
	return key, nil
}
