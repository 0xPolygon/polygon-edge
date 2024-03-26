package leveldb

import (
	"context"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/blockchain/storagev2"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
)

func newStorage(t *testing.T) (*storagev2.Storage, func()) {
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

func newStorageP(t *testing.T) (*storagev2.Storage, func(), string) {
	t.Helper()

	p, err := os.MkdirTemp("", "leveldbV2-test")
	require.NoError(t, err)

	require.NoError(t, os.MkdirAll(p, 0755))

	s, err := NewLevelDBStorage(p, hclog.NewNullLogger())
	require.NoError(t, err)

	closeFn := func() {
		require.NoError(t, s.Close())

		if err := s.Close(); err != nil {
			t.Fatal(err)
		}

		require.NoError(t, os.RemoveAll(p))
	}

	return s, closeFn, p
}

func countLdbFilesInPath(path string) int {
	pattern := filepath.Join(path, "*.ldb")

	files, err := filepath.Glob(pattern)
	if err != nil {
		return -1
	}

	return len(files)
}

func dirSize(t *testing.T, path string) int64 {
	t.Helper()

	var size int64

	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			t.Fail()
		}

		if !info.IsDir() {
			size += info.Size()
		}

		return err
	})
	if err != nil {
		t.Log(err)
	}

	return size
}

func TestStorage(t *testing.T) {
	storagev2.TestStorage(t, newStorage)
}

func TestWriteFullBlock(t *testing.T) {
	s, _, path := newStorageP(t)
	defer s.Close()

	count := 100
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*45)

	signchan := make(chan os.Signal, 1)
	signal.Notify(signchan, syscall.SIGINT)

	go func() {
		<-signchan
		cancel()
	}()

	blockchain := make(chan *types.FullBlock, 1)
	go storagev2.GenerateBlocks(t, count, blockchain, ctx)

insertloop:
	for i := 1; i <= count; i++ {
		select {
		case <-ctx.Done():
			break insertloop
		case b := <-blockchain:
			batchWriter := s.NewWriter()

			batchWriter.PutBody(b.Block.Number(), b.Block.Hash(), b.Block.Body())

			for _, tx := range b.Block.Transactions {
				batchWriter.PutTxLookup(tx.Hash(), b.Block.Number())
			}

			batchWriter.PutHeader(b.Block.Header)
			batchWriter.PutHeadNumber(uint64(i))
			batchWriter.PutHeadHash(b.Block.Header.Hash)
			batchWriter.PutReceipts(b.Block.Number(), b.Block.Hash(), b.Receipts)
			batchWriter.PutCanonicalHash(uint64(i), b.Block.Hash())
			require.NoError(t, batchWriter.WriteBatch())

			size := dirSize(t, path)
			t.Logf("writing block %d", i)
			t.Logf("\tldb file count: %d", countLdbFilesInPath(path))
			t.Logf("\tdir size %d MBs", size/1_000_000)
		}
	}
}
