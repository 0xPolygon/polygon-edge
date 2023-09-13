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
	return []byte{0x1}, true
}

func (m *mockSnapshot) Commit(objs []*Object) (Snapshot, *types.Trace, []byte) {
	return nil, nil, nil
}

func newStateWithPreState(preState map[types.Address]*PreState) Snapshot {
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

	assert.NoError(t, txn.RevertToSnapshot(ss))
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

	txn.GetCode(addr)

	txn.TouchAccount(addr)
	require.Len(t, txn.journal, 18)

	trace := txn.GetCompactJournal()
	require.Len(t, trace, 1)

	nonce := uint64(2)

	require.Equal(t, &types.JournalEntry{
		Balance: big.NewInt(2),
		Nonce:   &nonce,
		Storage: map[types.Hash]types.Hash{
			types.ZeroHash: oneHash,
			oneHash:        types.ZeroHash,
		},
		CodeRead: []byte{0x1},
		Touched:  boolTruePtr(),
		Read:     boolTruePtr(),
	}, trace[addr])
}

func TestJournalEntry_Merge(t *testing.T) {
	one := uint64(1)

	entryAllSet := func() *types.JournalEntry {
		// use a function because the merge function
		// modifies the caller and the test would
		// have side effects.
		return &types.JournalEntry{
			Nonce:   &one,
			Balance: big.NewInt(1),
			Storage: map[types.Hash]types.Hash{
				types.ZeroHash: types.ZeroHash,
			},
			Code:     []byte{0x1},
			CodeRead: []byte{0x1},
			Suicide:  boolTruePtr(),
			Touched:  boolTruePtr(),
			Read:     boolTruePtr(),
			StorageRead: map[types.Hash]struct{}{
				types.ZeroHash: {},
			},
		}
	}

	cases := []struct {
		a, b, c *types.JournalEntry // a.merge(b) = c
	}{
		{
			&types.JournalEntry{},
			entryAllSet(),
			entryAllSet(),
		},
		{
			entryAllSet(),
			&types.JournalEntry{},
			entryAllSet(),
		},
	}

	for _, c := range cases {
		c.a.Merge(c.b)
		require.Equal(t, c.c, c.a)
	}
}
