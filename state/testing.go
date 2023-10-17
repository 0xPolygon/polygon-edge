package state

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/0xPolygon/polygon-edge/types"
)

var (
	addr1 = types.StringToAddress("1")
	addr2 = types.StringToAddress("2")

	hash0 = types.StringToHash("0")
	hash1 = types.StringToHash("1")
	hash2 = types.StringToHash("2")

	defaultPreState = map[types.Address]*PreState{
		addr1: {
			State: map[types.Hash]types.Hash{
				hash1: hash1,
			},
		},
	}
)

// PreState is the account prestate
type PreState struct {
	Nonce   uint64
	Balance uint64
	State   map[types.Hash]types.Hash
}

// PreStates is a set of pre states
type PreStates map[types.Address]*PreState

type buildPreState func(p PreStates) Snapshot

// TestState tests a set of tests on a state
func TestState(t *testing.T, buildPreState buildPreState) {
	t.Helper()
	t.Parallel()

	t.Run("write state", func(t *testing.T) {
		t.Parallel()

		testWriteState(t, buildPreState)
	})
	t.Run("write empty state", func(t *testing.T) {
		t.Parallel()

		testWriteEmptyState(t, buildPreState)
	})
	t.Run("update state with empty - delete empty objects", func(t *testing.T) {
		t.Parallel()

		testUpdateStateWithEmpty(t, buildPreState, true)
	})
	t.Run("update state with empty - do not delete empty objects", func(t *testing.T) {
		t.Parallel()

		testUpdateStateWithEmpty(t, buildPreState, false)
	})
	t.Run("suicide account in pre-state", func(t *testing.T) {
		t.Parallel()

		testSuicideAccountInPreState(t, buildPreState)
	})
	t.Run("suicide account", func(t *testing.T) {
		t.Parallel()

		testSuicideAccount(t, buildPreState)
	})
	t.Run("suicide account with data", func(t *testing.T) {
		t.Parallel()

		testSuicideAccountWithData(t, buildPreState)
	})
	t.Run("suicide coinbase", func(t *testing.T) {
		t.Parallel()

		testSuicideCoinbase(t, buildPreState)
	})
	t.Run("suicide with intermediate commit", func(t *testing.T) {
		t.Parallel()

		testSuicideWithIntermediateCommit(t, buildPreState)
	})
	t.Run("restart refunds", func(t *testing.T) {
		t.Parallel()

		testRestartRefunds(t, buildPreState)
	})
	t.Run("change pre-state account balance to zero", func(t *testing.T) {
		t.Parallel()

		testChangePrestateAccountBalanceToZero(t, buildPreState)
	})
	t.Run("change account balance to zero", func(t *testing.T) {
		t.Parallel()

		testChangeAccountBalanceToZero(t, buildPreState)
	})
	t.Run("delete common state root", func(t *testing.T) {
		t.Parallel()

		testDeleteCommonStateRoot(t, buildPreState)
	})
	t.Run("get code empty code hash", func(t *testing.T) {
		t.Parallel()

		testGetCodeEmptyCodeHash(t, buildPreState)
	})
	t.Run("set and get code", func(t *testing.T) {
		t.Parallel()

		testSetAndGetCode(t, buildPreState)
	})
}

func testDeleteCommonStateRoot(t *testing.T, buildPreState buildPreState) {
	t.Helper()

	snap := buildPreState(nil)
	txn := newTxn(snap)

	txn.SetNonce(addr1, 1)
	txn.SetState(addr1, hash0, hash1)
	txn.SetState(addr1, hash1, hash1)
	txn.SetState(addr1, hash2, hash1)

	txn.SetNonce(addr2, 1)
	txn.SetState(addr2, hash0, hash1)
	txn.SetState(addr2, hash1, hash1)
	txn.SetState(addr2, hash2, hash1)

	objs, err := txn.Commit(false)
	require.NoError(t, err)

	snap2, _, err := snap.Commit(objs)
	require.NoError(t, err)

	txn2 := newTxn(snap2)

	txn2.SetState(addr1, hash0, hash0)
	txn2.SetState(addr1, hash1, hash0)

	objs, err = txn2.Commit(false)
	require.NoError(t, err)

	snap3, _, err := snap2.Commit(objs)
	require.NoError(t, err)

	txn3 := newTxn(snap3)
	assert.Equal(t, hash1, txn3.GetState(addr1, hash2))
	assert.Equal(t, hash1, txn3.GetState(addr2, hash0))
	assert.Equal(t, hash1, txn3.GetState(addr2, hash1))
	assert.Equal(t, hash1, txn3.GetState(addr2, hash2))
}

func testWriteState(t *testing.T, buildPreState buildPreState) {
	t.Helper()

	snap := buildPreState(nil)
	txn := newTxn(snap)

	txn.SetState(addr1, hash1, hash1)
	txn.SetState(addr1, hash2, hash2)

	assert.Equal(t, hash1, txn.GetState(addr1, hash1))
	assert.Equal(t, hash2, txn.GetState(addr1, hash2))

	objs, err := txn.Commit(false)
	require.NoError(t, err)

	snap, _, err = snap.Commit(objs)
	require.NoError(t, err)

	txn = newTxn(snap)
	assert.Equal(t, hash1, txn.GetState(addr1, hash1))
	assert.Equal(t, hash2, txn.GetState(addr1, hash2))
}

func testWriteEmptyState(t *testing.T, buildPreState buildPreState) {
	t.Helper()
	// Create account and write empty state
	snap := buildPreState(nil)
	txn := newTxn(snap)

	// Without EIP150 the data is added
	txn.SetState(addr1, hash1, hash0)

	objs, err := txn.Commit(false)
	require.NoError(t, err)

	snap, _, err = snap.Commit(objs)
	require.NoError(t, err)

	txn = newTxn(snap)
	assert.True(t, txn.Exist(addr1))

	snap = buildPreState(nil)
	txn = newTxn(snap)

	// With EIP150 the empty data is removed
	txn.SetState(addr1, hash1, hash0)

	objs, err = txn.Commit(true)
	require.NoError(t, err)

	snap, _, err = snap.Commit(objs)
	require.NoError(t, err)

	txn = newTxn(snap)
	assert.False(t, txn.Exist(addr1))
}

func testUpdateStateWithEmpty(t *testing.T, buildPreState buildPreState, deleteEmptyObjects bool) {
	t.Helper()

	// If the state (in prestate) is updated to empty it should be removed
	snap := buildPreState(defaultPreState)

	txn := newTxn(snap)
	txn.SetState(addr1, hash1, hash0)

	objs, err := txn.Commit(deleteEmptyObjects)
	require.NoError(t, err)

	snap, _, err = snap.Commit(objs)
	require.NoError(t, err)

	txn = newTxn(snap)
	assert.Equal(t, !deleteEmptyObjects, txn.Exist(addr1))

	// if balance is set, no matter what is passed to deleteEmptyObjects,
	// addr1 should exist in state objects of txn
	txn.SetBalance(addr1, big.NewInt(100))

	objs, err = txn.Commit(deleteEmptyObjects)
	require.NoError(t, err)

	snap, _, err = snap.Commit(objs)
	require.NoError(t, err)

	txn = newTxn(snap)
	assert.Equal(t, true, txn.Exist(addr1))

	// if nonce is set, no matter what is passed to deleteEmptyObjects,
	// addr1 should exist in state objects of txn
	txn.SetBalance(addr1, big.NewInt(0))
	txn.SetNonce(addr1, 1)

	objs, err = txn.Commit(deleteEmptyObjects)
	require.NoError(t, err)

	snap, _, err = snap.Commit(objs)
	require.NoError(t, err)

	txn = newTxn(snap)
	assert.Equal(t, true, txn.Exist(addr1))
}

func testSuicideAccountInPreState(t *testing.T, buildPreState buildPreState) {
	t.Helper()

	// Suicide an account created in the prestate
	snap := buildPreState(defaultPreState)

	txn := newTxn(snap)
	txn.Suicide(addr1)

	objs, err := txn.Commit(true)
	require.NoError(t, err)

	snap, _, err = snap.Commit(objs)
	require.NoError(t, err)

	txn = newTxn(snap)
	assert.False(t, txn.Exist(addr1))
}

func testSuicideAccount(t *testing.T, buildPreState buildPreState) {
	t.Helper()
	// Create a new account and suicide it
	snap := buildPreState(nil)

	txn := newTxn(snap)
	txn.SetState(addr1, hash1, hash1)
	txn.Suicide(addr1)

	// Note, even if has commit suicide it still exists in the current txn
	assert.True(t, txn.Exist(addr1))

	objs, err := txn.Commit(true)
	require.NoError(t, err)

	snap, _, err = snap.Commit(objs)
	require.NoError(t, err)

	txn = newTxn(snap)
	assert.False(t, txn.Exist(addr1))
}

func testSuicideAccountWithData(t *testing.T, buildPreState buildPreState) {
	t.Helper()
	// Data (nonce, balance, code) from a suicided account should be empty
	snap := buildPreState(nil)

	txn := newTxn(snap)

	code := []byte{0x1, 0x2, 0x3}

	txn.SetNonce(addr1, 10)
	txn.SetBalance(addr1, big.NewInt(100))
	txn.SetCode(addr1, code)
	txn.SetState(addr1, hash1, hash1)

	txn.Suicide(addr1)

	objs, err := txn.Commit(true)
	require.NoError(t, err)

	snap, _, err = snap.Commit(objs)
	require.NoError(t, err)

	txn = newTxn(snap)

	assert.Equal(t, big.NewInt(0), txn.GetBalance(addr1))
	assert.Equal(t, uint64(0), txn.GetNonce(addr1))

	// code is not yet on the state
	assert.Nil(t, txn.GetCode(addr1))
	assert.Equal(t, (types.Hash{}), txn.GetCodeHash(addr1))
	assert.Equal(t, int(0), txn.GetCodeSize(addr1))

	assert.Equal(t, (types.Hash{}), txn.GetState(addr1, hash1))
}

func testSuicideCoinbase(t *testing.T, buildPreState buildPreState) {
	t.Helper()
	// Suicide the coinbase of the block
	snap := buildPreState(defaultPreState)

	txn := newTxn(snap)
	txn.Suicide(addr1)
	txn.AddSealingReward(addr1, big.NewInt(10))
	objs, err := txn.Commit(true)
	require.NoError(t, err)

	snap, _, err = snap.Commit(objs)
	require.NoError(t, err)

	txn = newTxn(snap)
	assert.Equal(t, big.NewInt(10), txn.GetBalance(addr1))
}

func testSuicideWithIntermediateCommit(t *testing.T, buildPreState buildPreState) {
	t.Helper()

	snap := buildPreState(defaultPreState)

	txn := newTxn(snap)
	txn.SetNonce(addr1, 10)
	txn.Suicide(addr1)

	assert.Equal(t, uint64(10), txn.GetNonce(addr1))

	assert.NoError(t, txn.CleanDeleteObjects(true))
	assert.Equal(t, uint64(0), txn.GetNonce(addr1))

	_, err := txn.Commit(true)
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), txn.GetNonce(addr1))
}

func testRestartRefunds(t *testing.T, buildPreState buildPreState) {
	t.Helper()
	// refunds are only valid per single txn so after each
	// intermediateCommit they have to be restarted
	snap := buildPreState(nil)

	txn := newTxn(snap)

	txn.AddRefund(1000)
	assert.Equal(t, uint64(1000), txn.GetRefund())

	_, err := txn.Commit(false)
	assert.NoError(t, err)

	// refund should be empty after the commit
	assert.Equal(t, uint64(0), txn.GetRefund())
}

func testChangePrestateAccountBalanceToZero(t *testing.T, buildPreState buildPreState) {
	t.Helper()
	// If the balance of the account changes to zero the account is deleted
	preState := map[types.Address]*PreState{
		addr1: {
			Balance: 10,
		},
	}

	snap := buildPreState(preState)

	txn := newTxn(snap)
	txn.SetBalance(addr1, big.NewInt(0))

	objs, err := txn.Commit(true)
	require.NoError(t, err)

	snap, _, err = snap.Commit(objs)
	require.NoError(t, err)

	txn = newTxn(snap)
	assert.False(t, txn.Exist(addr1))
}

func testChangeAccountBalanceToZero(t *testing.T, buildPreState buildPreState) {
	t.Helper()
	// If the balance of the account changes to zero the account is deleted
	snap := buildPreState(nil)

	txn := newTxn(snap)
	txn.SetBalance(addr1, big.NewInt(10))
	txn.SetBalance(addr1, big.NewInt(0))

	objs, err := txn.Commit(true)
	require.NoError(t, err)

	snap, _, err = snap.Commit(objs)
	require.NoError(t, err)

	txn = newTxn(snap)
	assert.False(t, txn.Exist(addr1))
}

func testGetCodeEmptyCodeHash(t *testing.T, buildPreState buildPreState) {
	t.Helper()

	// If empty code hash is passed, it is considered as a valid case,
	// and in that case we are not retrieving it from the storage.
	snap := buildPreState(nil)

	code, ok := snap.GetCode(types.EmptyCodeHash)
	assert.True(t, ok)
	assert.Empty(t, code)
}

func testSetAndGetCode(t *testing.T, buildPreState buildPreState) {
	t.Helper()

	testCode := []byte{0x2, 0x4, 0x6, 0x8}
	snap := buildPreState(nil)

	txn := newTxn(snap)
	txn.SetCode(addr1, testCode)

	affectedObjs, err := txn.Commit(true)
	require.NoError(t, err)

	snap, _, err = snap.Commit(affectedObjs)
	require.NoError(t, err)

	assert.Len(t, affectedObjs, 1)

	code, ok := snap.GetCode(affectedObjs[0].CodeHash)
	assert.True(t, ok)
	assert.Equal(t, testCode, code)
}
