package memory

import (
	"testing"

	"github.com/umbracle/minimal/blockchain/storage"
)

func TestStorage(t *testing.T) {
	s, _ := NewMemoryStorage(nil)
	storage.TestStorage(t, s)
}
