package minimal

import (
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/ethereum/go-ethereum/crypto"
)

// Keystore stores the key for the enode
type Keystore interface {
	Put(*ecdsa.PrivateKey) error
	Get() (*ecdsa.PrivateKey, error)
}

// LocalKeystore loads the key from a local file
type LocalKeystore struct {
	path string
}

func (k *LocalKeystore) Put(key *ecdsa.PrivateKey) error {
	err := ioutil.WriteFile(k.path, []byte(hex.EncodeToString(crypto.FromECDSA(key))), 0600)
	return err
}

func (k *LocalKeystore) Get() (*ecdsa.PrivateKey, bool, error) {
	_, err := os.Stat(k.path)
	if err != nil && !os.IsNotExist(err) {
		return nil, false, fmt.Errorf("Failed to stat (%s): %v", k.path, err)
	}
	if os.IsNotExist(err) {
		return nil, false, nil
	}

	// exists
	data, err := ioutil.ReadFile(k.path)
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
