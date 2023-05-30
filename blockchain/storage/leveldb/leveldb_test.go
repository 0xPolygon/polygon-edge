package leveldb

import (
	"context"
	"fmt"
	"math/big"
	"math/rand"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/blockchain"
	"github.com/0xPolygon/polygon-edge/blockchain/storage"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
)

func newStorage(t *testing.T) (storage.Storage, func()) {
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

func TestStorage(t *testing.T) {
	storage.TestStorage(t, newStorage)
}

func generateRandomByteSlice(count int) []byte {
	s := make([]byte, count)
	for i := 0; i < count; i++ {
		s[i] = byte(rand.Int())
	}

	return s
}

func generateFullTx(nonce uint64, from types.Address, to *types.Address, value *big.Int, dynamic bool, v *big.Int) *types.Transaction {
	tx := &types.Transaction{}

	tx.Gas = types.StateTransactionGasLimit
	tx.Nonce = nonce
	tx.From = from
	tx.To = to
	tx.Value = value
	tx.V = v
	tx.Input = generateRandomByteSlice(1000)
	tx.Hash = types.BytesToHash(generateRandomByteSlice(32))

	if dynamic {
		tx.Type = types.DynamicFeeTx
		tx.GasFeeCap = v
		tx.GasTipCap = v
	} else {
		tx.Type = types.LegacyTx
		tx.GasPrice = v
	}

	return tx
}

func generateFullTxs(t *testing.T, startNonce, count int, from types.Address, to *types.Address) []*types.Transaction {
	t.Helper()

	v := big.NewInt(1)
	txs := make([]*types.Transaction, count)

	for i := 0; i < count; i++ {
		txs[i] = generateFullTx(uint64(startNonce+i), from, to, big.NewInt(1), false, v)
	}

	return txs
}

var (
	addr1 = types.StringToAddress("1")
	addr2 = types.StringToAddress("2")
)

func generateBlock(t *testing.T, num uint64) *types.FullBlock {
	t.Helper()

	b := &types.FullBlock{}

	b.Block = &types.Block{}
	b.Block.Header = &types.Header{
		Number:    num,
		ExtraData: generateRandomByteSlice(32),
		Hash:      types.BytesToHash(generateRandomByteSlice(32)),
	}

	b.Block.Transactions = generateFullTxs(t, 0, 2500, addr1, &addr2)
	b.Receipts = make([]*types.Receipt, len(b.Block.Transactions))
	b.Block.Uncles = blockchain.NewTestHeaders(10)

	var status types.ReceiptStatus = types.ReceiptSuccess

	logs := make([]*types.Log, 10)

	for i := 0; i < 10; i++ {
		logs[i] = &types.Log{
			Address: addr1,
			Topics:  []types.Hash{types.StringToHash("topic1"), types.StringToHash("topic2"), types.StringToHash("topic3")},
			Data:    []byte{0xaa, 0xbb, 0xcc, 0xdd, 0xbb, 0xaa, 0x01, 0x012},
		}
	}

	for i := 0; i < len(b.Block.Transactions); i++ {
		b.Receipts[i] = &types.Receipt{
			TxHash:            b.Block.Transactions[i].Hash,
			Root:              types.StringToHash("mockhashstring"),
			TransactionType:   types.LegacyTx,
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

func newStorageP(t *testing.T) (storage.Storage, func(), string) {
	t.Helper()

	p, err := os.MkdirTemp("", "leveldbtest")
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

func generateBlocks(t *testing.T, count int, ch chan *types.FullBlock, ctx context.Context) {
	t.Helper()

	ticker := time.NewTicker(time.Second)

	for i := 1; i <= count; i++ {
		b := generateBlock(t, uint64(i))
		select {
		case <-ctx.Done():
			close(ch)
			ticker.Stop()

			return
		case <-ticker.C:
			ch <- b
		}
	}
}

func DirSize(path string) (int64, error) {
	var size int64

	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}

		return err
	})

	return size, err
}

func TestWriteFullBlock(t *testing.T) {
	t.Helper()

	s, _, path := newStorageP(t)
	defer s.Close()

	count := 40000
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*45)

	signchan := make(chan os.Signal, 1)
	signal.Notify(signchan, syscall.SIGINT)

	go func() {
		<-signchan
		cancel()
	}()

	blockchain := make(chan *types.FullBlock, 1)
	go generateBlocks(t, count, blockchain, ctx)

insertloop:
	for i := 1; i <= count; i++ {
		select {
		case <-ctx.Done():
			break insertloop
		case b := <-blockchain:
			batch := s.NewBatch()
			batchHelper := storage.NewBatchHelper(batch)

			batchHelper.WriteBody(b.Block.Hash(), b.Block.Body())

			for _, tx := range b.Block.Transactions {
				batchHelper.WriteTxLookup(tx.Hash, b.Block.Hash())
			}

			batchHelper.WriteHeader(b.Block.Header)
			batchHelper.WriteHeadNumber(uint64(i))
			batchHelper.WriteHeadHash(b.Block.Header.Hash)
			batchHelper.WriteReceipts(b.Block.Hash(), b.Receipts)

			if err := batch.Write(); err != nil {
				fmt.Println(err)
				t.FailNow()
			}

			fmt.Println("writing block", i)

			size, _ := DirSize(path)
			fmt.Println("\tldb file count:", countLdbFilesInPath(path))
			fmt.Println("\tdir size", size/1_000_000, "MBs")
		}
	}
}
