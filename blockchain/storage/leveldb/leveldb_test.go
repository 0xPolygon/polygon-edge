package leveldb

import (
	"os"
	"testing"

	"github.com/hashicorp/go-hclog"
	"go.uber.org/goleak"

	"github.com/0xPolygon/polygon-edge/blockchain/storage"
)

func newStorage(t *testing.T) (storage.Storage, func()) {
	t.Helper()

	path, err := os.MkdirTemp("/tmp", "minimal_storage")
	if err != nil {
		t.Fatal(err)
	}

	s, err := NewLevelDBStorage(path, hclog.NewNullLogger())
	if err != nil {
		t.Fatal(err)
	}

	closeFn := func() {
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}

		if err := os.RemoveAll(path); err != nil {
			t.Fatal(err)
		}
	}

	return s, closeFn
}

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestStorage(t *testing.T) {
	storage.TestStorage(t, newStorage)
}
