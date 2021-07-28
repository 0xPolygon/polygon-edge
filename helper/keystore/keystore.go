package keystore

import (
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"os"
)

type createFn func() ([]byte, error)

type readFn func(b []byte) (interface{}, error)

// CreateIfNotExists generates a private key at the specified path,
// or reads it if a key file is present
func CreateIfNotExists(path string, create createFn, read readFn) (interface{}, error) {
	_, err := os.Stat(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to stat (%s): %v", path, err)
	}

	var keyBuff []byte
	if os.IsNotExist(err) {
		// Key doesn't exist yet

		// Create a key and write it to disk as base64
		keyBuff, err = create()
		if err != nil {
			return nil, fmt.Errorf("unable to generate private key, %v", err)
		}

		// Encode it to a readable format (Base64)
		keyBuff = []byte(hex.EncodeToString(keyBuff))
		if err = ioutil.WriteFile(path, keyBuff, 0600); err != nil {
			return nil, fmt.Errorf("unable to write private key to disk (%s), %v", path, err)
		}
	} else {
		// Key exists

		// Read the Base64 encoded key from the disk.
		// Base64 decoding is done in the read method
		keyBuff, err = ioutil.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("unable to read private key from disk (%s), %v", path, err)
		}
	}

	obj, err := read(keyBuff)
	if err != nil {
		return nil, fmt.Errorf("unable to execute byte array -> private key conversion, %v", err)
	}

	return obj, nil
}
