package keystore

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
)

func TestLocalKeystore(t *testing.T) {
	dir, err := ioutil.TempDir("/tmp", "test-localkeystore-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	keystore := LocalKeystore{
		Path: filepath.Join(dir, "key"),
	}

	key, exists, err := keystore.Get()
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("it should not exist")
	}

	newKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}

	if err := keystore.Put(newKey); err != nil {
		t.Fatal(err)
	}

	key, exists, err = keystore.Get()
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("it should exist")
	}

	if !reflect.DeepEqual(key, newKey) {
		t.Fatal("bad")
	}
}
