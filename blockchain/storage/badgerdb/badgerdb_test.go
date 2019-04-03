package badgerdb

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/umbracle/minimal/blockchain/storage"
)

func newStorage(t *testing.T) (storage.Storage, func()) {
	path, err := ioutil.TempDir("/tmp", "minimal_storage")
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewBadgerDBStorage(path, nil)
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
	s, close := newStorage(t)
	defer close()

	storage.TestStorage(t, s)
}
