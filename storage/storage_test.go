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
	s, err := NewStorage(path)
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
		if err := s.WriteCanonicalHash(cc.Number, cc.Hash); err != nil {
			t.Fatal(err)
		}

		data, err := s.ReadCanonicalHash(cc.Number)
		if err != nil {
			t.Fatal(err)
		}

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
		if err := s.WriteDiff(cc.Hash, cc.Diff); err != nil {
			t.Fatal(err)
		}
		diff, err := s.ReadDiff(cc.Hash)
		if err != nil {
			t.Fatal(err)
		}
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
		if err := s.WriteHeadHash(cc.Hash); err != nil {
			t.Fatal(err)
		}
		hash, err := s.ReadHeadHash()
		if err != nil {
			t.Fatal(err)
		}
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
		if err := s.WriteForks(cc.Forks); err != nil {
			t.Fatal(err)
		}
		forks, err := s.ReadForks()
		if err != nil {
			t.Fatal(err)
		}
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

	if err := s.WriteHeader(header); err != nil {
		t.Fatal(err)
	}
	header1, err := s.ReadHeader(header.Hash())
	if err != nil {
		t.Fatal(err)
	}

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

	if err := s.WriteBody(hash, block.Body()); err != nil {
		t.Fatal(err)
	}
	body, err := s.ReadBody(hash)
	if err != nil {
		t.Fatal(err)
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

// TODO, not working
func TestReceipts(t *testing.T) {
	s, close := newStorage(t)
	defer close()

	r0 := types.NewReceipt([]byte{1}, false, 10)
	r0.TxHash = common.HexToHash("11")

	r1 := types.NewReceipt([]byte{1}, false, 10)
	r1.TxHash = common.HexToHash("33")

	receipts := []*types.Receipt{r0, r1}
	hash := common.HexToHash("11")

	if err := s.WriteReceipts(hash, receipts); err != nil {
		t.Fatal(err)
	}

	r, err := s.ReadReceipts(hash)
	if err != nil {
		t.Fatal(err)
	}

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
