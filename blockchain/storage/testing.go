package storage

import (
	"bytes"
	"math/big"
	"reflect"
	"testing"

	"github.com/umbracle/minimal/types"
)

// TestStorage tests a set of tests on a storage
func TestStorage(t *testing.T, s Storage) {
	t.Helper()

	t.Run("", func(t *testing.T) {
		testCanonicalChain(t, s)
	})
	t.Run("", func(t *testing.T) {
		testDifficulty(t, s)
	})
	t.Run("", func(t *testing.T) {
		testHead(t, s)
	})
	t.Run("", func(t *testing.T) {
		testForks(t, s)
	})
	t.Run("", func(t *testing.T) {
		testHeader(t, s)
	})
	t.Run("", func(t *testing.T) {
		testBody(t, s)
	})
	t.Run("", func(t *testing.T) {
		testReceipts(t, s)
	})
}

func testCanonicalChain(t *testing.T, s Storage) {
	var cases = []struct {
		Number uint64
		Hash   types.Hash
	}{
		{
			Number: 1,
			Hash:   types.StringToHash("111"),
		},
		{
			Number: 1,
			Hash:   types.StringToHash("222"),
		},
		{
			Number: 2,
			Hash:   types.StringToHash("111"),
		},
	}

	for _, cc := range cases {
		s.WriteCanonicalHash(cc.Number, cc.Hash)
		data, ok := s.ReadCanonicalHash(cc.Number)
		if !ok {
			t.Fatal("not found")
		}
		if !reflect.DeepEqual(data, cc.Hash) {
			t.Fatal("not match")
		}
	}
}

func testDifficulty(t *testing.T, s Storage) {
	var cases = []struct {
		Hash types.Hash
		Diff *big.Int
	}{
		{
			Hash: types.StringToHash("0x1"),
			Diff: big.NewInt(10),
		},
		{
			Hash: types.StringToHash("0x1"),
			Diff: big.NewInt(11),
		},
		{
			Hash: types.StringToHash("0x2"),
			Diff: big.NewInt(12),
		},
	}

	for _, cc := range cases {
		s.WriteDiff(cc.Hash, cc.Diff)
		diff, ok := s.ReadDiff(cc.Hash)
		if !ok {
			t.Fatal("not found")
		}
		if !reflect.DeepEqual(cc.Diff, diff) {
			t.Fatal("bad")
		}
	}
}

func testHead(t *testing.T, s Storage) {
	var cases = []struct {
		Hash types.Hash
	}{
		{types.StringToHash("111")},
		{types.StringToHash("222")},
		{types.StringToHash("222")},
	}

	for _, cc := range cases {
		s.WriteHeadHash(cc.Hash)
		hash, ok := s.ReadHeadHash()
		if !ok {
			t.Fatal("not found")
		}
		if !reflect.DeepEqual(cc.Hash, hash) {
			t.Fatal("bad")
		}
	}
}

func testForks(t *testing.T, s Storage) {
	var cases = []struct {
		Forks []types.Hash
	}{
		{[]types.Hash{types.StringToHash("111"), types.StringToHash("222")}},
		{[]types.Hash{types.StringToHash("111")}},
	}

	for _, cc := range cases {
		s.WriteForks(cc.Forks)
		forks := s.ReadForks()

		if !reflect.DeepEqual(cc.Forks, forks) {
			t.Fatal("bad")
		}
	}
}

func testHeader(t *testing.T, s Storage) {
	header := &types.Header{
		Number:     5,
		Difficulty: 10,
		ParentHash: types.StringToHash("11"),
		Timestamp:  10,
		ExtraData:  []byte{}, // if not set it will fail
	}

	s.WriteHeader(header)
	header1, ok := s.ReadHeader(header.Hash())
	if !ok {
		t.Fatal("not found")
	}
	if !reflect.DeepEqual(header.Hash(), header1.Hash()) {
		t.Fatal("bad")
	}
}

func testBody(t *testing.T, s Storage) {
	header := &types.Header{
		Number:     5,
		Difficulty: 10,
		ParentHash: types.StringToHash("11"),
		Timestamp:  10,
		ExtraData:  []byte{}, // if not set it will fail
	}

	addr1 := types.StringToAddress("11")
	t0 := &types.Transaction{
		Nonce:    0,
		To:       &addr1,
		Value:    big.NewInt(1).Bytes(),
		Gas:      11,
		GasPrice: big.NewInt(11).Bytes(),
		Input:    []byte{1, 2},
	}

	addr2 := types.StringToAddress("22")
	t1 := &types.Transaction{
		Nonce:    0,
		To:       &addr2,
		Value:    big.NewInt(1).Bytes(),
		Gas:      22,
		GasPrice: big.NewInt(11).Bytes(),
		Input:    []byte{4, 5},
	}

	block := types.Block{
		Header:       header,
		Transactions: []*types.Transaction{t0, t1},
	}
	hash := block.Hash()

	body0 := block.Body()
	s.WriteBody(hash, body0)

	body1, ok := s.ReadBody(hash)
	if !ok {
		t.Fatal("not found")
	}

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

func testReceipts(t *testing.T, s Storage) {

	r0 := &types.Receipt{
		Root:              []byte{1},
		Status:            types.ReceiptFailed,
		CumulativeGasUsed: 10,
		TxHash:            types.StringToHash("11"),
	}

	r1 := &types.Receipt{
		Root:              []byte{1},
		Status:            types.ReceiptFailed,
		CumulativeGasUsed: 10,
		TxHash:            types.StringToHash("33"),
	}

	receipts := []*types.Receipt{r0, r1}
	hash := types.StringToHash("11")

	s.WriteReceipts(hash, receipts)
	r := s.ReadReceipts(hash)

	// NOTE: reflect.DeepEqual does not seem to work, check the hash of the receipt
	if len(r) != len(receipts) {
		t.Fatal("lengths are different")
	}
	for indx, i := range receipts {
		if !bytes.Equal(i.Root, r[indx].Root) {
			t.Fatal("receipt txhash is not correct")
		}
	}
}
