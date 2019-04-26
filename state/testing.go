package state

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
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

// PreState is the account prestate
type PreState struct {
	Nonce   uint64
	Balance uint64
	State   map[common.Hash]common.Hash
}

// PreStates is a set of pre states
type PreStates map[common.Address]*PreState

type buildPreState func(p PreStates) (State, Snapshot)

// TestState tests a set of tests on a state
func TestState(t *testing.T, buildPreState buildPreState) {
	t.Helper()

	t.Run("", func(t *testing.T) {
		testWriteState(t, buildPreState)
	})
	t.Run("", func(t *testing.T) {
		testWriteEmptyState(t, buildPreState)
	})
	t.Run("", func(t *testing.T) {
		testUpdateStateInPreState(t, buildPreState)
	})
	t.Run("", func(t *testing.T) {
		testUpdateStateWithEmpty(t, buildPreState)
	})
	t.Run("", func(t *testing.T) {
		testSuicideAccountInPreState(t, buildPreState)
	})
	t.Run("", func(t *testing.T) {
		testSuicideAccount(t, buildPreState)
	})
	t.Run("", func(t *testing.T) {
		testSuicideAccountWithData(t, buildPreState)
	})
	t.Run("", func(t *testing.T) {
		testSuicideCoinbase(t, buildPreState)
	})
	t.Run("", func(t *testing.T) {
		testSuicideWithIntermediateCommit(t, buildPreState)
	})
	t.Run("", func(t *testing.T) {
		testRestartRefunds(t, buildPreState)
	})
	t.Run("", func(t *testing.T) {
		testChangePrestateAccountBalanceToZero(t, buildPreState)
	})
	t.Run("", func(t *testing.T) {
		testChangeAccountBalanceToZero(t, buildPreState)
	})
}

func testWriteState(t *testing.T, buildPreState buildPreState) {
	state, snap := buildPreState(nil)
	txn := newTxn(state, snap)

	txn.SetState(addr1, hash1, hash1)
	txn.SetState(addr1, hash2, hash2)

	assert.Equal(t, hash1, txn.GetState(addr1, hash1))
	assert.Equal(t, hash2, txn.GetState(addr1, hash2))

	snap, _ = txn.Commit(false)

	txn = newTxn(state, snap)
	assert.Equal(t, hash1, txn.GetState(addr1, hash1))
	assert.Equal(t, hash2, txn.GetState(addr1, hash2))
}

func testWriteEmptyState(t *testing.T, buildPreState buildPreState) {
	// Create account and write empty state
	state, snap := buildPreState(nil)
	txn := newTxn(state, snap)

	// Without EIP150 the data is added
	txn.SetState(addr1, hash1, hash0)
	snap, _ = txn.Commit(false)

	txn = newTxn(state, snap)
	assert.True(t, txn.Exist(addr1))

	_, snap = buildPreState(nil)
	txn = newTxn(state, snap)

	// With EIP150 the empty data is removed
	txn.SetState(addr1, hash1, hash0)
	snap, _ = txn.Commit(true)

	txn = newTxn(state, snap)
	assert.False(t, txn.Exist(addr1))
}

func testUpdateStateInPreState(t *testing.T, buildPreState buildPreState) {
	// update state that was already set in prestate
	state, snap := buildPreState(defaultPreState)

	txn := newTxn(state, snap)
	assert.Equal(t, hash1, txn.GetState(addr1, hash1))

	txn.SetState(addr1, hash1, hash2)
	snap, _ = txn.Commit(false)

	txn = newTxn(state, snap)
	assert.Equal(t, hash2, txn.GetState(addr1, hash1))
}

func testUpdateStateWithEmpty(t *testing.T, buildPreState buildPreState) {
	// If the state (in prestate) is updated to empty it should be removed
	state, snap := buildPreState(defaultPreState)

	txn := newTxn(state, snap)
	txn.SetState(addr1, hash1, hash0)

	// TODO, test with false (should not be deleted)
	// TODO, test with balance on the account and nonce
	snap, _ = txn.Commit(true)

	txn = newTxn(state, snap)
	assert.False(t, txn.Exist(addr1))
}

func testSuicideAccountInPreState(t *testing.T, buildPreState buildPreState) {
	// Suicide an account created in the prestate
	state, snap := buildPreState(defaultPreState)

	txn := newTxn(state, snap)
	txn.Suicide(addr1)
	snap, _ = txn.Commit(true)

	txn = newTxn(state, snap)
	assert.False(t, txn.Exist(addr1))
}

func testSuicideAccount(t *testing.T, buildPreState buildPreState) {
	// Create a new account and suicide it
	state, snap := buildPreState(nil)

	txn := newTxn(state, snap)
	txn.SetState(addr1, hash1, hash1)
	txn.Suicide(addr1)

	// Note, even if has commit suicide it still exists in the current txn
	assert.True(t, txn.Exist(addr1))

	snap, _ = txn.Commit(true)

	txn = newTxn(state, snap)
	assert.False(t, txn.Exist(addr1))
}

func testSuicideAccountWithData(t *testing.T, buildPreState buildPreState) {
	// Data (nonce, balance, code) from a suicided account should be empty
	state, snap := buildPreState(nil)

	txn := newTxn(state, snap)

	code := []byte{0x1, 0x2, 0x3}
	txn.SetNonce(addr1, 10)
	txn.SetBalance(addr1, big.NewInt(100))
	txn.SetCode(addr1, code)
	txn.SetState(addr1, hash1, hash1)

	txn.Suicide(addr1)
	snap, _ = txn.Commit(true)

	txn = newTxn(state, snap)

	assert.Equal(t, big.NewInt(0), txn.GetBalance(addr1))
	assert.Equal(t, uint64(0), txn.GetNonce(addr1))

	// code is not yet on the state
	assert.Nil(t, txn.GetCode(addr1))
	assert.Equal(t, (common.Hash{}), txn.GetCodeHash(addr1))
	assert.Equal(t, int(0), txn.GetCodeSize(addr1))

	assert.Equal(t, (common.Hash{}), txn.GetState(addr1, hash1))
}

func testSuicideCoinbase(t *testing.T, buildPreState buildPreState) {
	// Suicide the coinbase of the block
	state, snap := buildPreState(defaultPreState)

	txn := newTxn(state, snap)
	txn.Suicide(addr1)
	txn.AddSealingReward(addr1, big.NewInt(10))
	snap, _ = txn.Commit(true)

	txn = newTxn(state, snap)
	assert.Equal(t, big.NewInt(10), txn.GetBalance(addr1))
}

func testSuicideWithIntermediateCommit(t *testing.T, buildPreState buildPreState) {
	state, snap := buildPreState(defaultPreState)

	txn := newTxn(state, snap)
	txn.SetNonce(addr1, 10)
	txn.Suicide(addr1)

	assert.Equal(t, uint64(10), txn.GetNonce(addr1))

	txn.cleanDeleteObjects(true)
	assert.Equal(t, uint64(0), txn.GetNonce(addr1))

	txn.Commit(true)
	assert.Equal(t, uint64(0), txn.GetNonce(addr1))
}

func testRestartRefunds(t *testing.T, buildPreState buildPreState) {
	// refunds are only valid per single txn so after each
	// intermediateCommit they have to be restarted
	state, snap := buildPreState(nil)

	txn := newTxn(state, snap)

	txn.AddRefund(1000)
	assert.Equal(t, uint64(1000), txn.GetRefund())

	txn.Commit(false)

	// refund should be empty after the commit
	assert.Equal(t, uint64(0), txn.GetRefund())
}

func testChangePrestateAccountBalanceToZero(t *testing.T, buildPreState buildPreState) {
	// If the balance of the account changes to zero the account is deleted
	preState := map[common.Address]*PreState{
		addr1: {
			Balance: 10,
		},
	}

	state, snap := buildPreState(preState)

	txn := newTxn(state, snap)
	txn.SetBalance(addr1, big.NewInt(0))
	snap, _ = txn.Commit(true)

	txn = newTxn(state, snap)
	assert.False(t, txn.Exist(addr1))
}

func testChangeAccountBalanceToZero(t *testing.T, buildPreState buildPreState) {
	// If the balance of the account changes to zero the account is deleted
	state, snap := buildPreState(nil)

	txn := newTxn(state, snap)
	txn.SetBalance(addr1, big.NewInt(10))
	txn.SetBalance(addr1, big.NewInt(0))
	snap, _ = txn.Commit(true)

	txn = newTxn(state, snap)
	assert.False(t, txn.Exist(addr1))
}
