package memory

import (
	"testing"

	"go.uber.org/goleak"

	"github.com/0xPolygon/polygon-edge/blockchain/storage"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestStorage(t *testing.T) {
	t.Helper()

	f := func(t *testing.T) (storage.Storage, func()) {
		t.Helper()

		s, _ := NewMemoryStorage(nil)

		return s, func() {}
	}
	storage.TestStorage(t, f)
}
