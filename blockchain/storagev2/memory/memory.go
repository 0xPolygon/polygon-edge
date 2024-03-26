package memory

import (
	"github.com/0xPolygon/polygon-edge/blockchain/storagev2"
	"github.com/0xPolygon/polygon-edge/helper/hex"
)

type memoryKV struct {
	kv map[string][]byte
}
type memoryDB struct {
	db []memoryKV
}

// NewMemoryStorage creates the new storage reference with inmemory
func NewMemoryStorage() (*storagev2.Storage, error) {
	var ldbs [2]storagev2.Database

	kvs := []memoryKV{}

	for i := 0; uint8(i) < storagev2.MAX_TABLES; i++ {
		kvs = append(kvs, memoryKV{kv: map[string][]byte{}})
	}

	db := &memoryDB{db: kvs}

	ldbs[0] = db
	ldbs[1] = nil

	return storagev2.Open(nil, ldbs)
}

func (m *memoryDB) Get(t uint8, k []byte) ([]byte, bool, error) {
	v, ok := m.db[t].kv[hex.EncodeToHex(k)]
	if !ok {
		return nil, false, nil
	}

	return v, true, nil
}

func (m *memoryDB) Close() error {
	return nil
}

func (m *memoryDB) NewBatch() storagev2.Batch {
	return newBatchMemory(m.db)
}
