package state

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	trie "github.com/umbracle/minimal/state/immutable-trie"
)

var addr1 = common.HexToAddress("1")
var addr2 = common.HexToAddress("2")

var hash0 = common.HexToHash("0")
var hash1 = common.HexToHash("1")
var hash2 = common.HexToHash("2")

var defaultPreState = map[common.Address]*PreState{
	addr1: {
		State: map[common.Hash]common.Hash{
			hash1: hash1,
		},
	},
}

func buildPreState(s *Snapshot, preState map[common.Address]*PreState) *Snapshot {
	txn := s.Txn()
	for i, j := range preState {
		txn.SetNonce(i, j.Nonce)
		txn.SetBalance(i, big.NewInt(int64(j.Balance)))
		for k, v := range j.State {
			txn.SetState(i, k, v)
		}
	}
	s, _ = txn.Commit(false)
	return s
}

type PreState struct {
	Nonce   uint64
	Balance uint64
	State   map[common.Hash]common.Hash
}

func TestWriteState(t *testing.T) {
	// write new state

	s := NewState(trie.NewState(trie.NewMemoryStorage()))
	snap, _ := s.NewSnapshot(common.Hash{})
	txn := snap.Txn()

	txn.SetState(addr1, hash1, hash1)
	txn.SetState(addr1, hash2, hash2)

	if txn.GetState(addr1, hash1) != hash1 {
		t.Fatal()
	}
	if txn.GetState(addr1, hash2) != hash2 {
		t.Fatal()
	}

	snap, _ = txn.Commit(false)

	txn = snap.Txn()
	if txn.GetState(addr1, hash1) != hash1 {
		t.Fatal()
	}
	if txn.GetState(addr1, hash2) != hash2 {
		t.Fatal()
	}
}

func TestWriteEmptyState(t *testing.T) {
	// Create account and write empty state

	s := NewState(trie.NewState(trie.NewMemoryStorage()))
	snap, _ := s.NewSnapshot(common.Hash{})
	txn := snap.Txn()

	// Without EIP150 the data is added
	txn.SetState(addr1, hash1, hash0)
	snap, _ = txn.Commit(false)

	txn = snap.Txn()
	if !txn.Exist(addr1) {
		t.Fatal()
	}

	s = NewState(trie.NewState(trie.NewMemoryStorage()))
	snap, _ = s.NewSnapshot(common.Hash{})

	txn = snap.Txn()

	// With EIP150 the empty data is removed
	txn.SetState(addr1, hash1, hash0)
	snap, _ = txn.Commit(true)

	txn = snap.Txn()
	if txn.Exist(addr1) {
		t.Fatal()
	}
}

func TestUpdateStateInPreState(t *testing.T) {
	// update state that was already set in prestate
	s := NewState(trie.NewState(trie.NewMemoryStorage()))
	snap, _ := s.NewSnapshot(common.Hash{})

	snap = buildPreState(snap, defaultPreState)

	txn := snap.Txn()
	if txn.GetState(addr1, hash1) != hash1 {
		t.Fatal()
	}

	txn.SetState(addr1, hash1, hash2)
	snap, _ = txn.Commit(false)

	txn = snap.Txn()
	if txn.GetState(addr1, hash1) != hash2 {
		t.Fatal()
	}
}

func TestUpdateStateWithEmpty(t *testing.T) {
	// If the state (in prestate) is updated to empty it should be removed
	s := NewState(trie.NewState(trie.NewMemoryStorage()))
	snap, _ := s.NewSnapshot(common.Hash{})

	snap = buildPreState(snap, defaultPreState)

	txn := snap.Txn()
	txn.SetState(addr1, hash1, hash0)

	// TODO, test with false (should not be deleted)
	// TODO, test with balance on the account and nonce
	snap, _ = txn.Commit(true)

	txn = snap.Txn()
	if txn.Exist(addr1) {
		t.Fatal()
	}
}

func TestSuicideAccountInPreState(t *testing.T) {
	// Suicide an account created in the prestate
	s := NewState(trie.NewState(trie.NewMemoryStorage()))
	snap, _ := s.NewSnapshot(common.Hash{})
	snap = buildPreState(snap, defaultPreState)

	txn := snap.Txn()
	txn.Suicide(addr1)
	snap, _ = txn.Commit(true)

	txn = snap.Txn()
	if txn.Exist(addr1) {
		t.Fatal()
	}
}

func TestSuicideAccount(t *testing.T) {
	// Create a new account and suicide it
	s := NewState(trie.NewState(trie.NewMemoryStorage()))
	snap, _ := s.NewSnapshot(common.Hash{})

	txn := snap.Txn()
	txn.SetState(addr1, hash1, hash1)
	txn.Suicide(addr1)

	// Note, even if has commit suicide it still exists in the current txn
	if !txn.Exist(addr1) {
		t.Fatal()
	}

	snap, _ = txn.Commit(true)

	txn = snap.Txn()
	if txn.Exist(addr1) {
		t.Fatal()
	}
}

func TestSuicideAccountWithData(t *testing.T) {
	// Data (nonce, balance, code) from a suicided account should be empty
	s := NewState(trie.NewState(trie.NewMemoryStorage()))
	snap, _ := s.NewSnapshot(common.Hash{})

	txn := snap.Txn()

	txn.SetNonce(addr1, 10)
	txn.SetBalance(addr1, big.NewInt(100))
	txn.SetCode(addr1, []byte{0x1, 0x2, 0x3})
	txn.SetState(addr1, hash1, hash1)

	txn.Suicide(addr1)
	snap, _ = txn.Commit(true)

	txn = snap.Txn()

	if balance := txn.GetBalance(addr1); balance.Cmp(big.NewInt(0)) != 0 {
		t.Fatalf("balance should be zero but found: %d", balance)
	}
	if nonce := txn.GetNonce(addr1); nonce != 0 {
		t.Fatalf("nonce should be zero but found %d", nonce)
	}
	if code := txn.GetCode(addr1); len(code) != 0 {
		t.Fatalf("code should be empty but found: %s", hexutil.Encode(code))
	}
	if codeHash := txn.GetCodeHash(addr1); codeHash != (common.Hash{}) {
		t.Fatalf("code hash should be empty but found: %s", codeHash.String())
	}
	if size := txn.GetCodeSize(addr1); size != 0 {
		t.Fatalf("code size should be zero but found %d", size)
	}
	if value := txn.GetState(addr1, hash1); value != (common.Hash{}) {
		t.Fatalf("value should be empty but found: %s", value.String())
	}
}

func TestSuicideCoinbase(t *testing.T) {
	// Suicide the coinbase of the block
	s := NewState(trie.NewState(trie.NewMemoryStorage()))
	snap, _ := s.NewSnapshot(common.Hash{})
	snap = buildPreState(snap, defaultPreState)

	txn := snap.Txn()
	txn.Suicide(addr1)
	txn.AddSealingReward(addr1, big.NewInt(10))
	snap, _ = txn.Commit(true)

	txn = snap.Txn()
	if txn.GetBalance(addr1).Cmp(big.NewInt(10)) != 0 {
		t.Fatal()
	}
}

func TestSuicideWithIntermediateCommit(t *testing.T) {
	// Legacy
	s := NewState(trie.NewState(trie.NewMemoryStorage()))
	snap, _ := s.NewSnapshot(common.Hash{})
	snap = buildPreState(snap, defaultPreState)

	txn := snap.Txn()
	txn.SetNonce(addr1, 10)
	txn.Suicide(addr1)

	if txn.GetNonce(addr1) != 10 { // it is still 'active'
		t.Fatal()
	}

	txn.cleanDeleteObjects(true)

	if txn.GetNonce(addr1) == 10 {
		t.Fatal()
	}

	txn.Commit(true)
	if txn.GetNonce(addr1) == 10 {
		t.Fatal()
	}
}

func TestRestartRefunds(t *testing.T) {
	// refunds are only valid per single txn so after each
	// intermediateCommit they have to be restarted
	s := NewState(trie.NewState(trie.NewMemoryStorage()))
	snap, _ := s.NewSnapshot(common.Hash{})

	txn := snap.Txn()

	txn.AddRefund(1000)
	if txn.GetRefund() != 1000 {
		t.Fatal()
	}

	txn.Commit(false)
	if refunds := txn.GetRefund(); refunds == 1000 {
		t.Fatalf("refunds should be empty buf founds: %d", refunds)
	}
}

func TestChangePrestateAccountBalanceToZero(t *testing.T) {
	// If the balance of the account changes to zero the account is deleted
	preState := map[common.Address]*PreState{
		addr1: {
			Balance: 10,
		},
	}

	s := NewState(trie.NewState(trie.NewMemoryStorage()))
	snap, _ := s.NewSnapshot(common.Hash{})
	snap = buildPreState(snap, preState)

	txn := snap.Txn()
	txn.SetBalance(addr1, big.NewInt(0))
	snap, _ = txn.Commit(true)

	txn = snap.Txn()
	if txn.Exist(addr1) {
		t.Fatal()
	}
}

func TestChangeAccountBalanceToZero(t *testing.T) {
	// If the balance of the account changes to zero the account is deleted
	s := NewState(trie.NewState(trie.NewMemoryStorage()))
	snap, _ := s.NewSnapshot(common.Hash{})

	txn := snap.Txn()
	txn.SetBalance(addr1, big.NewInt(10))
	txn.SetBalance(addr1, big.NewInt(0))
	snap, _ = txn.Commit(true)

	txn = snap.Txn()
	if txn.Exist(addr1) {
		t.Fatal()
	}
}

func TestSnapshotUpdateData(t *testing.T) {
	// Snapshots should keep the data

	s := NewState(trie.NewState(trie.NewMemoryStorage()))
	snap, _ := s.NewSnapshot(common.Hash{})

	txn := snap.Txn()

	txn.SetState(addr1, hash1, hash1)
	if txn.GetState(addr1, hash1) != hash1 {
		t.Fail()
	}

	ss := txn.Snapshot()
	txn.SetState(addr1, hash1, hash2)
	if txn.GetState(addr1, hash1) != hash2 {
		t.Fail()
	}

	txn.RevertToSnapshot(ss)
	if txn.GetState(addr1, hash1) != hash1 {
		t.Fail()
	}
}
