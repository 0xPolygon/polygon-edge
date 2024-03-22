package storagev2

import (
	"context"
	"crypto/rand"
	"math/big"
	"reflect"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type PlaceholderStorage func(t *testing.T) (*Storage, func())

var (
	addr1 = types.StringToAddress("1")
	addr2 = types.StringToAddress("2")

	hash1 = types.StringToHash("1")
	hash2 = types.StringToHash("2")
)

// TestStorage tests a set of tests on a storage
func TestStorage(t *testing.T, m PlaceholderStorage) {
	t.Helper()

	t.Run("testCanonicalChain", func(t *testing.T) {
		testCanonicalChain(t, m)
	})
	t.Run("testDifficulty", func(t *testing.T) {
		testDifficulty(t, m)
	})
	t.Run("testHead", func(t *testing.T) {
		testHead(t, m)
	})
	t.Run("testForks", func(t *testing.T) {
		testForks(t, m)
	})
	t.Run("testHeader", func(t *testing.T) {
		testHeader(t, m)
	})
	t.Run("testBody", func(t *testing.T) {
		testBody(t, m)
	})
	t.Run("testWriteCanonicalHeader", func(t *testing.T) {
		testWriteCanonicalHeader(t, m)
	})
	t.Run("testReceipts", func(t *testing.T) {
		testReceipts(t, m)
	})
}

func testCanonicalChain(t *testing.T, m PlaceholderStorage) {
	t.Helper()

	s, closeFn := m(t)
	defer closeFn()

	var cases = []struct {
		Number     uint64
		ParentHash types.Hash
		Hash       types.Hash
	}{
		{
			Number:     1,
			ParentHash: types.StringToHash("111"),
		},
		{
			Number:     1,
			ParentHash: types.StringToHash("222"),
		},
		{
			Number:     2,
			ParentHash: types.StringToHash("111"),
		},
	}

	for _, cc := range cases {
		batch := s.NewWriter()

		h := &types.Header{
			Number:     cc.Number,
			ParentHash: cc.ParentHash,
			ExtraData:  []byte{0x1},
		}

		hash := h.Hash

		batch.PutHeader(h)
		batch.PutCanonicalHash(cc.Number, hash)

		require.NoError(t, batch.WriteBatch())

		data, ok := s.ReadCanonicalHash(cc.Number)
		assert.True(t, ok)

		if !reflect.DeepEqual(data, hash) {
			t.Fatal("not match")
		}
	}
}

func testDifficulty(t *testing.T, m PlaceholderStorage) {
	t.Helper()

	s, closeFn := m(t)
	defer closeFn()

	var cases = []struct {
		Diff *big.Int
	}{
		{
			Diff: big.NewInt(10),
		},
		{
			Diff: big.NewInt(11),
		},
		{
			Diff: big.NewInt(12),
		},
	}

	for indx, cc := range cases {
		batch := s.NewWriter()

		h := &types.Header{
			Number:    uint64(indx),
			ExtraData: []byte{},
		}
		h.ComputeHash()

		batch.PutHeader(h)
		batch.PutBlockLookup(h.Hash, h.Number)
		batch.PutTotalDifficulty(h.Number, h.Hash, cc.Diff)

		require.NoError(t, batch.WriteBatch())

		diff, ok := s.ReadTotalDifficulty(h.Number, h.Hash)
		assert.True(t, ok)

		if !reflect.DeepEqual(cc.Diff, diff) {
			t.Fatal("bad")
		}
	}
}

func testHead(t *testing.T, m PlaceholderStorage) {
	t.Helper()

	s, closeFn := m(t)
	defer closeFn()

	for i := uint64(0); i < 5; i++ {
		batch := s.NewWriter()

		h := &types.Header{
			Number:    i,
			ExtraData: []byte{},
		}
		hash := h.Hash

		batch.PutHeader(h)
		batch.PutHeadNumber(i)
		batch.PutHeadHash(hash)

		require.NoError(t, batch.WriteBatch())

		n2, ok := s.ReadHeadNumber()
		assert.True(t, ok)

		if n2 != i {
			t.Fatal("bad")
		}

		hash1, ok := s.ReadHeadHash()
		assert.True(t, ok)

		if !reflect.DeepEqual(hash1, hash) {
			t.Fatal("bad")
		}
	}
}

func testForks(t *testing.T, m PlaceholderStorage) {
	t.Helper()

	s, closeFn := m(t)
	defer closeFn()

	var cases = []struct {
		Forks []types.Hash
	}{
		{[]types.Hash{types.StringToHash("111"), types.StringToHash("222")}},
		{[]types.Hash{types.StringToHash("111")}},
	}

	for _, cc := range cases {
		batch := s.NewWriter()

		batch.PutForks(cc.Forks)

		require.NoError(t, batch.WriteBatch())

		forks, err := s.ReadForks()
		assert.NoError(t, err)

		if !reflect.DeepEqual(cc.Forks, forks) {
			t.Fatal("bad")
		}
	}
}

func testHeader(t *testing.T, m PlaceholderStorage) {
	t.Helper()

	s, closeFn := m(t)
	defer closeFn()

	header := &types.Header{
		Number:     5,
		Difficulty: 17179869184,
		ParentHash: types.StringToHash("11"),
		Timestamp:  10,
		// if not set it will fail
		ExtraData: hex.MustDecodeHex("0x11bbe8db4e347b4e8c937c1c8370e4b5ed33adb3db69cbdb7a38e1e50b1b82fa"),
	}
	header.ComputeHash()

	batch := s.NewWriter()

	batch.PutHeader(header)

	require.NoError(t, batch.WriteBatch())

	header1, err := s.ReadHeader(header.Number, header.Hash)
	assert.NoError(t, err)

	if !reflect.DeepEqual(header, header1) {
		t.Fatal("bad")
	}
}

func testBody(t *testing.T, m PlaceholderStorage) {
	t.Helper()

	s, closeFn := m(t)
	defer closeFn()

	header := &types.Header{
		Number:     5,
		Difficulty: 10,
		ParentHash: types.StringToHash("11"),
		Timestamp:  10,
		ExtraData:  []byte{}, // if not set it will fail
	}

	header.ComputeHash()

	batch := s.NewWriter()

	batch.PutHeader(header)

	require.NoError(t, batch.WriteBatch())

	addr1 := types.StringToAddress("11")
	t0 := types.NewTx(types.NewLegacyTx(
		types.WithNonce(0),
		types.WithTo(&addr1),
		types.WithValue(big.NewInt(1)),
		types.WithGas(11),
		types.WithGasPrice(big.NewInt(11)),
		types.WithInput([]byte{1, 2}),
		types.WithSignatureValues(big.NewInt(1), nil, nil),
	))
	t0.ComputeHash()

	addr2 := types.StringToAddress("22")
	t1 := types.NewTx(types.NewLegacyTx(
		types.WithNonce(0),
		types.WithTo(&addr2),
		types.WithValue(big.NewInt(1)),
		types.WithGas(22),
		types.WithGasPrice(big.NewInt(11)),
		types.WithInput([]byte{4, 5}),
		types.WithSignatureValues(big.NewInt(2), nil, nil),
	))
	t1.ComputeHash()

	block := types.Block{
		Header:       header,
		Transactions: []*types.Transaction{t0, t1},
	}

	batch2 := s.NewWriter()
	body0 := block.Body()

	batch2.PutBody(header.Number, header.Hash, body0)

	require.NoError(t, batch2.WriteBatch())

	body1, err := s.ReadBody(header.Number, header.Hash)
	assert.NoError(t, err)

	// NOTE: reflect.DeepEqual does not seem to work, check the hash of the transactions
	tx0, tx1 := body0.Transactions, body1.Transactions
	if len(tx0) != len(tx1) {
		t.Fatal("lengths are different")
	}

	for indx, i := range tx0 {
		if i.Hash() != tx1[indx].Hash() {
			t.Fatal("tx not correct")
		}
	}
}

func testReceipts(t *testing.T, m PlaceholderStorage) {
	t.Helper()

	s, closeFn := m(t)
	defer closeFn()

	batch := s.NewWriter()

	h := &types.Header{
		Difficulty: 133,
		Number:     11,
		ExtraData:  []byte{},
	}
	h.ComputeHash()

	body := &types.Body{
		Transactions: []*types.Transaction{
			types.NewTx(types.NewStateTx(
				types.WithNonce(1000),
				types.WithGas(50),
				types.WithGasPrice(new(big.Int).SetUint64(100)),
				types.WithSignatureValues(big.NewInt(11), nil, nil),
			)),
		},
	}
	receipts := []*types.Receipt{
		{
			Root:              types.StringToHash("1"),
			CumulativeGasUsed: 10,
			TxHash:            body.Transactions[0].Hash(),
			LogsBloom:         types.Bloom{0x1},
			Logs: []*types.Log{
				{
					Address: addr1,
					Topics:  []types.Hash{hash1, hash2},
					Data:    []byte{0x1, 0x2},
				},
				{
					Address: addr2,
					Topics:  []types.Hash{hash1},
				},
			},
		},
		{
			Root:              types.StringToHash("1"),
			CumulativeGasUsed: 10,
			TxHash:            body.Transactions[0].Hash(),
			LogsBloom:         types.Bloom{0x1},
			GasUsed:           10,
			ContractAddress:   &types.Address{0x1},
			Logs: []*types.Log{
				{
					Address: addr2,
					Topics:  []types.Hash{hash1},
				},
			},
		},
	}

	batch.PutHeader(h)
	batch.PutBody(h.Number, h.Hash, body)
	batch.PutReceipts(h.Number, h.Hash, receipts)

	require.NoError(t, batch.WriteBatch())

	found, err := s.ReadReceipts(h.Number, h.Hash)
	assert.NoError(t, err)
	assert.True(t, reflect.DeepEqual(receipts, found))
}

func testWriteCanonicalHeader(t *testing.T, m PlaceholderStorage) {
	t.Helper()

	s, closeFn := m(t)
	defer closeFn()

	h := &types.Header{
		Number:    100,
		ExtraData: []byte{0x1},
	}
	h.ComputeHash()

	diff := new(big.Int).SetUint64(100)
	batch := s.NewWriter()

	batch.PutCanonicalHeader(h, diff)

	require.NoError(t, batch.WriteBatch())

	hh, err := s.ReadHeader(h.Number, h.Hash)
	assert.NoError(t, err)

	if !reflect.DeepEqual(h, hh) {
		t.Fatal("bad header")
	}

	headHash, ok := s.ReadHeadHash()
	assert.True(t, ok)

	if headHash != h.Hash {
		t.Fatal("head hash not correct")
	}

	headNum, ok := s.ReadHeadNumber()
	assert.True(t, ok)

	if headNum != h.Number {
		t.Fatal("head num not correct")
	}

	canHash, ok := s.ReadCanonicalHash(h.Number)
	assert.True(t, ok)

	if canHash != h.Hash {
		t.Fatal("canonical hash not correct")
	}
}

func generateTxs(t *testing.T, startNonce, count int, from types.Address, to *types.Address) []*types.Transaction {
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

		input := make([]byte, 1000)
		_, err := rand.Read(input)

		require.NoError(t, err)

		tx.ComputeHash()

		txs[i] = tx
	}

	return txs
}

func generateBlock(t *testing.T, num uint64) *types.FullBlock {
	t.Helper()

	transactionsCount := 2500
	status := types.ReceiptSuccess
	addr1 := types.StringToAddress("17878aa")
	addr2 := types.StringToAddress("2bf5653")
	b := &types.FullBlock{
		Block: &types.Block{
			Header: &types.Header{
				Number:    num,
				ExtraData: make([]byte, 32),
				Hash:      types.ZeroHash,
			},
			Transactions: generateTxs(t, 0, transactionsCount, addr1, &addr2),
			// Uncles:       blockchain.NewTestHeaders(10),
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

func GenerateBlocks(t *testing.T, count int, ch chan *types.FullBlock, ctx context.Context) {
	t.Helper()

	ticker := time.NewTicker(100 * time.Millisecond)

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
