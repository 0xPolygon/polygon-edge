package keystore

import (
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"os"
)

type createFn func() ([]byte, error)

type readFn func(b []byte) (interface{}, error)

func CreateIfNotExists(path string, create createFn, read readFn) (interface{}, error) {
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
