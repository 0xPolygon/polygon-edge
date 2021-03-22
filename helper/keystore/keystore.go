package keystore

import (
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/0xPolygon/minimal/crypto"
	lcrypto "github.com/libp2p/go-libp2p-core/crypto"
)

type createFn func() ([]byte, error)

type readFn func(b []byte) (interface{}, error)

func createIfNotExists(path string, create createFn, read readFn) (interface{}, error) {
	_, err := os.Stat(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to stat (%s): %v", path, err)
	}
	if os.IsNotExist(err) {
		// create the file
		buf, err := create()
		if err != nil {
			return nil, err
		}
		if err := ioutil.WriteFile(path, []byte(hex.EncodeToString(buf)), 0600); err != nil {
			return nil, err
		}
	}

	// read the file
	raw, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	obj, err := read(raw)
	if err != nil {
		return nil, err
	}
	return obj, nil
}

func ReadLibp2pKey(path string) (lcrypto.PrivKey, error) {
	createFn := func() ([]byte, error) {
		priv, _, err := lcrypto.GenerateKeyPair(lcrypto.Secp256k1, 256)
		if err != nil {
			return nil, err
		}
		buf, err := lcrypto.MarshalPrivateKey(priv)
		if err != nil {
			return nil, err
		}
		return buf, nil
	}
	readFn := func(b []byte) (interface{}, error) {
		buf, err := hex.DecodeString(string(b))
		if err != nil {
			return nil, err
		}
		key, err := lcrypto.UnmarshalPrivateKey(buf)
		if err != nil {
			return nil, err
		}
		return key, nil
	}
	obj, err := createIfNotExists(path, createFn, readFn)
	if err != nil {
		return nil, err
	}
	return obj.(lcrypto.PrivKey), nil
}

func ReadPrivKey(path string) (*ecdsa.PrivateKey, error) {
	createFn := func() ([]byte, error) {
		key, err := crypto.GenerateKey()
		if err != nil {
			return nil, err
		}
		buf, err := crypto.MarshallPrivateKey(key)
		if err != nil {
			return nil, err
		}
		return buf, nil
	}
	readFn := func(b []byte) (interface{}, error) {
		buf, err := ioutil.ReadFile(path)
		if err != nil {
			return nil, err
		}
		key, err := crypto.ParsePrivateKey(buf)
		if err != nil {
			return nil, err
		}
		return key, nil
	}
	obj, err := createIfNotExists(path, createFn, readFn)
	if err != nil {
		return nil, err
	}
	return obj.(*ecdsa.PrivateKey), nil
}
