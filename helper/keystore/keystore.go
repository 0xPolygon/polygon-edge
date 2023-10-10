package keystore

import (
	"encoding/hex"
	"fmt"
	"os"

	"github.com/0xPolygon/polygon-edge/helper/common"
)

type createFn func() ([]byte, error)

// CreateIfNotExists generates a private key at the specified path,
// or reads the file on that path if it is present
func CreateIfNotExists(path string, create createFn) ([]byte, error) {
	_, err := os.Stat(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to stat (%s): %w", path, err)
	}

	var keyBuff []byte
	if !os.IsNotExist(err) {
		// Key exists
		keyBuff, err = os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("unable to read private key from disk (%s), %w", path, err)
		}

		return keyBuff, nil
	}

	// Key doesn't exist yet, generate it
	keyBuff, err = create()
	if err != nil {
		return nil, fmt.Errorf("unable to generate private key, %w", err)
	}

	// Encode it to a readable format (hex) and write to disk
	keyBuff = []byte(hex.EncodeToString(keyBuff))
	if err = common.SaveFileSafe(path, keyBuff, 0440); err != nil {
		return nil, fmt.Errorf("unable to write private key to disk (%s), %w", path, err)
	}

	return keyBuff, nil
}

func CreatePrivateKey(create createFn) ([]byte, error) {
	keyBuff, err := create()
	if err != nil {
		return nil, fmt.Errorf("unable to generate private key, %w", err)
	}

	// Encode it to a readable format (hex) and return
	return []byte(hex.EncodeToString(keyBuff)), nil
}
