package memory

import (
	"github.com/0xPolygon/minimal/blockchain/storage"
	"github.com/0xPolygon/minimal/helper/hex"
	"github.com/hashicorp/go-hclog"
)

// NewMemoryStorage creates the new storage reference with inmemory
func NewMemoryStorage(logger hclog.Logger) (storage.Storage, error) {
	return storage.NewKeyValueStorage(logger, NewMemoryRaw()), nil
}

func NewMemoryRaw() storage.KV {
	db := &memoryKV{map[string][]byte{}}
	return db
}

// memoryKV is an in memory implementation of the kv storage
type memoryKV struct {
	db map[string][]byte
}

func (m *memoryKV) Set(p []byte, v []byte) error {
	m.db[hex.EncodeToHex(p)] = v
	return nil
}

func (m *memoryKV) Get(p []byte) ([]byte, bool, error) {
	v, ok := m.db[hex.EncodeToHex(p)]
	if !ok {
		return nil, false, nil
	}
	return v, true, nil
}

func (m *memoryKV) Close() error {
	return nil
}
