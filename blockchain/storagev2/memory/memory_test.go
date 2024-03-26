package memory

import (
	"testing"

	"github.com/0xPolygon/polygon-edge/blockchain/storagev2"
)

func TestStorage(t *testing.T) {
	t.Helper()

	f := func(t *testing.T) (*storagev2.Storage, func()) {
		t.Helper()

		s, err := NewMemoryStorage()

		if err != nil {
			t.Logf("\t Error opening MemoryDB -> %s", err.Error())

			return nil, func() {}
		}

		return s, func() {}
	}
	storagev2.TestStorage(t, f)
}
