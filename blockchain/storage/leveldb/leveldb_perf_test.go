package leveldb

import (
	"math/big"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/blockchain"
	"github.com/0xPolygon/polygon-edge/blockchain/storage"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
)

func createTxs(t *testing.T, startNonce, count int, from types.Address, to *types.Address) []*types.Transaction {
	t.Helper()

	txs := make([]*types.Transaction, count)

	for i := range txs {
		tx := types.NewTx(types.NewDynamicFeeTx(
			types.WithGas(types.StateTransactionGasLimit),
			types.WithNonce(uint64(startNonce+i)),
			types.WithFrom(from),
			types.WithTo(to),
			types.WithValue(big.NewInt(2000)),
			types.WithGasFeeCap(big.NewInt(100)),
			types.WithGasTipCap(big.NewInt(10)),
		))

		txs[i] = tx
	}

	return txs
}

const letterBytes = "abcdef0123456789"

func randStringBytes(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[rand.Intn(len(letterBytes))]
	}

	return string(b)
}

func createBlock(t *testing.T) *types.FullBlock {
	t.Helper()

	transactionsCount := 2500
	status := types.ReceiptSuccess
	addr1 := types.StringToAddress("17878aa")
	addr2 := types.StringToAddress("2bf5653")
	b := &types.FullBlock{
		Block: &types.Block{
			Header: &types.Header{
				Number:    0,
				ExtraData: make([]byte, 32),
				Hash:      types.ZeroHash,
			},
			Transactions: createTxs(t, 0, transactionsCount, addr1, &addr2),
			Uncles:       blockchain.NewTestHeaders(10),
		},
		Receipts: make([]*types.Receipt, transactionsCount),
	}

	logs := make([]*types.Log, 10)

	for i := 0; i < 10; i++ {
		logs[i] = &types.Log{
			Address: addr1,
			Topics:  []types.Hash{types.StringToHash("t1"), types.StringToHash("t2"), types.StringToHash("t3")},
			Data:    []byte{0xaa, 0xbb, 0xcc, 0xdd, 0xbb, 0xaa, 0x01, 0x012},
		}
	}

	for i := 0; i < len(b.Block.Transactions); i++ {
		b.Receipts[i] = &types.Receipt{
			TxHash:            b.Block.Transactions[i].Hash(),
			Root:              types.StringToHash("mockhashstring"),
			TransactionType:   types.LegacyTxType,
			GasUsed:           uint64(100000),
			Status:            &status,
			Logs:              logs,
			CumulativeGasUsed: uint64(100000),
			ContractAddress:   &types.Address{0xaa, 0xbb, 0xcc, 0xdd, 0xab, 0xac},
		}
	}

	for i := 0; i < 5; i++ {
		b.Receipts[i].LogsBloom = types.CreateBloom(b.Receipts)
	}

	return b
}

func openStorage(t *testing.T, p string) (storage.Storage, func(), string) {
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

func updateBlock(t *testing.T, num uint64, b *types.FullBlock) *types.FullBlock {
	t.Helper()

	var addr types.Address

	b.Block.Header.Number = num
	b.Block.Header.ParentHash = types.StringToHash(randStringBytes(12))

	for i := range b.Block.Transactions {
		addr = types.StringToAddress(randStringBytes(8))
		b.Block.Transactions[i].SetTo(&addr)
		b.Block.Transactions[i].ComputeHash()
		b.Receipts[i].TxHash = b.Block.Transactions[i].Hash()
	}

	b.Block.Header.ComputeHash()

	return b
}

func prepareBatch(t *testing.T, s storage.Storage, b *types.FullBlock) *storage.BatchWriter {
	t.Helper()

	batchWriter := storage.NewBatchWriter(s)

	// Lookup 'sorted'
	batchWriter.PutHeadHash(b.Block.Header.Hash)
	batchWriter.PutHeadNumber(b.Block.Number())

	for _, tx := range b.Block.Transactions {
		batchWriter.PutTxLookup(tx.Hash(), b.Block.Hash())
	}

	// Main DB sorted
	batchWriter.PutBody(b.Block.Hash(), b.Block.Body())
	batchWriter.PutCanonicalHash(b.Block.Number(), b.Block.Hash())
	batchWriter.PutHeader(b.Block.Header)
	batchWriter.PutReceipts(b.Block.Hash(), b.Receipts)

	return batchWriter
}

func TestWriteBlockPerf(t *testing.T) {
	t.SkipNow()

	s, _, path := openStorage(t, "/tmp/leveldbV1-test")
	defer s.Close()

	var watchTime int64

	count := 10000
	b := createBlock(t)

	for i := 1; i <= count; i++ {
		updateBlock(t, uint64(i), b)
		batchWriter := prepareBatch(t, s, b)

		tn := time.Now().UTC()

		if err := batchWriter.WriteBatch(); err != nil {
			require.NoError(t, err)
		}

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

	s, _, _ := openStorage(t, "/tmp/leveldbV1-test")
	defer s.Close()

	var watchTime int64

	count := 1000
	for i := 1; i <= count; i++ {
		n := uint64(1 + rand.Intn(10000))

		tn := time.Now().UTC()
		h, ok := s.ReadCanonicalHash(n)
		_, err2 := s.ReadBody(h)
		_, err3 := s.ReadHeader(h)
		_, err4 := s.ReadReceipts(h)
		d := time.Since(tn)

		watchTime += d.Milliseconds()

		if !ok || err2 != nil || err3 != nil || err4 != nil {
			t.Logf("\terror")
		}
	}
	t.Logf("\ttotal read %d ms", watchTime)
}
