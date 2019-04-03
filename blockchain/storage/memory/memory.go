package memory

import (
	"log"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/umbracle/minimal/blockchain/storage"
)

// NewMemoryStorage creates the new storage reference with inmemory
func NewMemoryStorage(logger *log.Logger) (storage.Storage, error) {
	db := &memoryKV{map[string][]byte{}}
	return storage.NewKeyValueStorage(logger, db), nil
}

// memoryKV is an in memory implementation of the kv storage
type memoryKV struct {
	db map[string][]byte
}

func (m *memoryKV) Set(p []byte, v []byte) error {
	m.db[hexutil.Encode(p)] = v
	return nil
}

func (m *memoryKV) Get(p []byte) ([]byte, bool, error) {
	v, ok := m.db[hexutil.Encode(p)]
	if !ok {
		return nil, false, nil
	}
	return v, true, nil
}
