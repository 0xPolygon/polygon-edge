package storage

import (
	"io/ioutil"
	"math/big"
	"os"
	"reflect"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

func newStorage(t *testing.T) (*Storage, func()) {
	path, err := ioutil.TempDir("/tmp", "minimal_storage")
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewStorage(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	close := func() {
		if err := os.RemoveAll(path); err != nil {
			t.Fatal(err)
		}
	}
	return s, close
}

func TestCanonicalChain(t *testing.T) {
	s, close := newStorage(t)
	defer close()

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
		data := s.ReadCanonicalHash(cc.Number)

		if !reflect.DeepEqual(data, cc.Hash) {
			t.Fatal("not match")
		}
	}
}

func TestDifficulty(t *testing.T) {
	s, close := newStorage(t)
	defer close()

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
		diff := s.ReadDiff(cc.Hash)

		if !reflect.DeepEqual(cc.Diff, diff) {
			t.Fatal("bad")
		}
	}
}

func TestHead(t *testing.T) {
	s, close := newStorage(t)
	defer close()

	var cases = []struct {
		Hash common.Hash
	}{
		{common.HexToHash("111")},
		{common.HexToHash("222")},
		{common.HexToHash("222")},
	}

	for _, cc := range cases {
		s.WriteHeadHash(cc.Hash)
		hash := s.ReadHeadHash()

		if !reflect.DeepEqual(cc.Hash, *hash) {
			t.Fatal("bad")
		}
	}
}

func TestForks(t *testing.T) {
	s, close := newStorage(t)
	defer close()

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

func TestHeader(t *testing.T) {
	s, close := newStorage(t)
	defer close()

	header := &types.Header{
		Number:     big.NewInt(5),
		Difficulty: big.NewInt(10),
		ParentHash: common.HexToHash("11"),
		Time:       big.NewInt(10),
		Extra:      []byte{}, // if not set it will fail
	}

	s.WriteHeader(header)
	header1 := s.ReadHeader(header.Hash())

	if !reflect.DeepEqual(header.Hash(), header1.Hash()) {
		t.Fatal("bad")
	}
}

func TestBody(t *testing.T) {
	s, close := newStorage(t)
	defer close()

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
	body := s.ReadBody(hash)

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

func TestReceipts(t *testing.T) {
	s, close := newStorage(t)
	defer close()

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

func TestEmptyData(t *testing.T) {
	s, close := newStorage(t)
	defer close()

	if s.ReadBody(common.HexToHash("1")) != nil {
		t.Fatal("body should be empty")
	}
	if s.ReadHeadHash() != nil {
		t.Fatal("head hash should be nil")
	}
	if s.ReadHeadNumber() != nil {
		t.Fatal("head hash should be nil")
	}
}
