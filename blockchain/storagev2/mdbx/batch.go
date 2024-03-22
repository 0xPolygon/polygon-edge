//nolint:errcheck
package mdbx

import (
	"runtime"

	"github.com/0xPolygon/polygon-edge/blockchain/storagev2"
	"github.com/erigontech/mdbx-go/mdbx"
)

type batchMdbx struct {
	tx  *mdbx.Txn
	dbi [storagev2.MAX_TABLES]mdbx.DBI
}

func newBatchMdbx(db *MdbxDB) *batchMdbx {
	runtime.LockOSThread()

	tx, err := db.env.BeginTxn(nil, 0)
	if err != nil {
		return nil
	}

	return &batchMdbx{
		tx:  tx,
		dbi: db.dbi,
	}
}

func (b *batchMdbx) Put(t uint8, k []byte, v []byte) {
	if t&storagev2.LOOKUP_INDEX != 0 {
		// Random write
		b.tx.Put(b.dbi[t], k, v, mdbx.NoDupData)
	} else {
		// Sequential write
		b.tx.Put(b.dbi[t], k, v, mdbx.Append) // Append at the end
	}
}

func (b *batchMdbx) Write() error {
	defer runtime.UnlockOSThread()

	_, err := b.tx.Commit()

	return err
}
