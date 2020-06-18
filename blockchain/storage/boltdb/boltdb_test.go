package boltdb

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/0xPolygon/minimal/blockchain/storage"
)

func newStorage(t *testing.T) (storage.Storage, func()) {
	path, err := ioutil.TempDir("/tmp", "minimal_storage")
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewBoltDBStorage(filepath.Join(path, "db"), nil)
	if err != nil {
		t.Fatal(err)
	}
	close := func() {
		if err := os.RemoveAll(path); err != nil {
			t.Fatal(err)
		}
	}
	return s, close
}

func TestStorage(t *testing.T) {
	storage.TestStorage(t, newStorage)
}
