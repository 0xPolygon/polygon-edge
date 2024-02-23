package memory

import (
	"github.com/0xPolygon/polygon-edge/blockchain/storagev2"
	"github.com/0xPolygon/polygon-edge/helper/hex"
)

type batchMemory struct {
	db          []memoryKV
	valuesToPut [storagev2.MAX_TABLES][][2][]byte
}

func newBatchMemory(db []memoryKV) *batchMemory {
	return &batchMemory{db: db}
}

func (b *batchMemory) Put(t uint8, k []byte, v []byte) {
	b.valuesToPut[t] = append(b.valuesToPut[t], [2][]byte{k, v})
}

func (b *batchMemory) Write() error {
	for i, j := range b.valuesToPut {
		for _, x := range j {
			b.db[i].kv[hex.EncodeToHex(x[0])] = x[1]
		}
	}

	return nil
}
