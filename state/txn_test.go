package state

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
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
