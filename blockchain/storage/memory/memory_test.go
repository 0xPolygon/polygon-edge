package memory

import (
	"testing"

	"go.uber.org/goleak"

	"github.com/0xPolygon/polygon-edge/blockchain/storage"
)

func TestStorage(t *testing.T) {
	t.Helper()
	defer goleak.VerifyNone(t)

	f := func(t *testing.T) (storage.Storage, func()) {
		t.Helper()
		defer goleak.VerifyNone(t)

		s, _ := NewMemoryStorage(nil)

		return s, func() {}
	}
	storage.TestStorage(t, f)
}
