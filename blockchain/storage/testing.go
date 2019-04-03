package storage

import (
	"math/big"
	"reflect"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
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
		Number *big.Int
		Hash   common.Hash
	}{
		{
			Number: big.NewInt(1),
			Hash:   common.HexToHash("111"),
		},
		{
			Number: big.NewInt(1),
			Hash:   common.HexToHash("222"),
		},
		{
			Number: big.NewInt(2),
			Hash:   common.HexToHash("111"),
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
		Hash common.Hash
		Diff *big.Int
	}{
		{
			Hash: common.HexToHash("0x1"),
			Diff: big.NewInt(10),
		},
		{
			Hash: common.HexToHash("0x1"),
			Diff: big.NewInt(11),
		},
		{
			Hash: common.HexToHash("0x2"),
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
		Hash common.Hash
	}{
		{common.HexToHash("111")},
		{common.HexToHash("222")},
		{common.HexToHash("222")},
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
		Forks []common.Hash
	}{
		{[]common.Hash{common.HexToHash("111"), common.HexToHash("222")}},
		{[]common.Hash{common.HexToHash("111")}},
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
		Number:     big.NewInt(5),
		Difficulty: big.NewInt(10),
		ParentHash: common.HexToHash("11"),
		Time:       big.NewInt(10),
		Extra:      []byte{}, // if not set it will fail
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
		Number:     big.NewInt(5),
		Difficulty: big.NewInt(10),
		ParentHash: common.HexToHash("11"),
		Time:       big.NewInt(10),
		Extra:      []byte{}, // if not set it will fail
	}

	t0 := types.NewTransaction(0, common.HexToAddress("11"), big.NewInt(1), 11, big.NewInt(11), []byte{1, 2})
	t1 := types.NewTransaction(1, common.HexToAddress("22"), big.NewInt(1), 22, big.NewInt(11), []byte{4, 5})

	block := types.NewBlock(header, []*types.Transaction{t0, t1}, nil, nil)
	hash := block.Hash()

	s.WriteBody(hash, block.Body())
	body, ok := s.ReadBody(hash)
	if !ok {
		t.Fatal("not found")
	}

	// NOTE: reflect.DeepEqual does not seem to work, check the hash of the transactions
	tx0, tx1 := block.Body().Transactions, body.Transactions
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
	r0 := types.NewReceipt([]byte{1}, false, 10)
	r0.TxHash = common.HexToHash("11")

	r1 := types.NewReceipt([]byte{1}, false, 10)
	r1.TxHash = common.HexToHash("33")

	receipts := []*types.Receipt{r0, r1}
	hash := common.HexToHash("11")

	s.WriteReceipts(hash, receipts)
	r := s.ReadReceipts(hash)

	// NOTE: reflect.DeepEqual does not seem to work, check the hash of the receipt
	if len(r) != len(receipts) {
		t.Fatal("lengths are different")
	}
	for indx, i := range receipts {
		if i.TxHash != r[indx].TxHash {
			t.Fatal("receipt txhash is not correct")
		}
	}
}
