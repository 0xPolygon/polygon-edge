package leveldb

import (
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/blockchain/storagev2"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openStorage(t *testing.T, p string) (*storagev2.Storage, func(), string) {
	t.Helper()

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

func dbSize(t *testing.T, path string) int64 {
	t.Helper()

	var size int64

	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			t.Fail()
		}

		if info != nil && !info.IsDir() && strings.Contains(info.Name(), ".ldb") {
			size += info.Size()
		}

		return err
	})
	if err != nil {
		t.Log(err)
	}

	return size
}

func TestWriteBlockPerf(t *testing.T) {
	t.SkipNow()

	s, _, path := openStorage(t, "/tmp/leveldbV2-test")
	defer s.Close()

	var watchTime int64

	count := 10000
	b := storagev2.CreateBlock(t)

	for i := 1; i <= count; i++ {
		storagev2.UpdateBlock(t, uint64(i), b)
		batchWriter := storagev2.PrepareBatch(t, s, b)

		tn := time.Now().UTC()

		require.NoError(t, batchWriter.WriteBatch())

		d := time.Since(tn)
		watchTime += d.Milliseconds()
	}

	time.Sleep(time.Second)

	size := dbSize(t, path)
	t.Logf("\tdb size %d MB", size/(1024*1024))
	t.Logf("\ttotal WriteBatch %d ms", watchTime)
}

func TestReadBlockPerf(t *testing.T) {
	t.SkipNow()

	s, _, _ := openStorage(t, "/tmp/leveldbV2-test")
	defer s.Close()

	var watchTime int64

	count := 1000
	for i := 1; i <= count; i++ {
		n := uint64(1 + rand.Intn(10000))

		tn := time.Now().UTC()
		h, ok := s.ReadCanonicalHash(n)
		_, err1 := s.ReadBody(n, h)
		_, err3 := s.ReadHeader(n, h)
		_, err4 := s.ReadReceipts(n, h)
		b, err5 := s.ReadBlockLookup(h)
		d := time.Since(tn)

		watchTime += d.Milliseconds()

		if !ok || err1 != nil || err3 != nil || err4 != nil || err5 != nil {
			t.Logf("\terror")
		}

		assert.Equal(t, n, b)
	}
	t.Logf("\ttotal read %d ms", watchTime)
}
