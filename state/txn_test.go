package state

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockSnapshot struct {
	state map[types.Address]*PreState
}

func (m *mockSnapshot) GetStorage(addr types.Address, root types.Hash, key types.Hash) types.Hash {
	raw, ok := m.state[addr]
	if !ok {
		return types.Hash{}
	}

	res, ok := raw.State[key]
	if !ok {
		return types.Hash{}
	}

	return res
}

func (m *mockSnapshot) GetAccount(addr types.Address) (*Account, error) {
	raw, ok := m.state[addr]
	if !ok {
		return nil, fmt.Errorf("account not found")
	}

	acct := &Account{
		Balance: new(big.Int).SetUint64(raw.Balance),
		Nonce:   raw.Nonce,
	}

	return acct, nil
}

func (m *mockSnapshot) GetCode(hash types.Hash) ([]byte, bool) {
	return nil, false
}

func newStateWithPreState(preState map[types.Address]*PreState) readSnapshot {
	return &mockSnapshot{state: preState}
}

func newTestTxn(p map[types.Address]*PreState) *Txn {
	return newTxn(newStateWithPreState(p))
}

func TestSnapshotUpdateData(t *testing.T) {
	txn := newTestTxn(defaultPreState)

	txn.SetState(addr1, hash1, hash1)
	assert.Equal(t, hash1, txn.GetState(addr1, hash1))

	ss := txn.Snapshot()
	txn.SetState(addr1, hash1, hash2)
	assert.Equal(t, hash2, txn.GetState(addr1, hash1))

	txn.RevertToSnapshot(ss)
	assert.Equal(t, hash1, txn.GetState(addr1, hash1))
}

func TestTxn_TracesCompaction(t *testing.T) {
	txn := newTestTxn(defaultPreState)

	addr := types.Address{}

	txn.SetBalance(addr, big.NewInt(1))
	txn.SetBalance(addr, big.NewInt(2)) // updates

	txn.SetNonce(addr, 1)
	txn.SetNonce(addr, 2) // updates

	oneHash := types.Hash{0x1}

	txn.SetState(addr, types.ZeroHash, types.ZeroHash)
	txn.SetState(addr, types.ZeroHash, oneHash) // updates
	txn.SetState(addr, oneHash, types.ZeroHash)

	trace := txn.getCompactJournal()
	require.Len(t, trace, 1)

	nonce := uint64(2)

	require.Equal(t, trace[addr], &journalEntry{
		Balance: big.NewInt(2),
		Nonce:   &nonce,
		Storage: map[types.Hash]types.Hash{
			types.ZeroHash: oneHash,
			oneHash:        types.ZeroHash,
		},
	})
}

func TestJournalEntry_Merge(t *testing.T) {
	one := uint64(1)
	boolTrue := true

	entryAllSet := func() *journalEntry {
		// use a function because the merge function
		// modifies the caller and the test would
		// have side effects.
		return &journalEntry{
			Nonce:   &one,
			Balance: big.NewInt(1),
			Storage: map[types.Hash]types.Hash{
				types.ZeroHash: types.ZeroHash,
			},
			Code:    []byte{0x1},
			Suicide: &boolTrue,
		}
	}

	cases := []struct {
		a, b, c *journalEntry // a.merge(b) = c
	}{
		{
			&journalEntry{},
			entryAllSet(),
			entryAllSet(),
		},
		{
			entryAllSet(),
			&journalEntry{},
			entryAllSet(),
		},
	}

	for _, c := range cases {
		c.a.merge(c.b)
		require.Equal(t, c.c, c.a)
	}
}
