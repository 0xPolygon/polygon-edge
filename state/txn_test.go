package state

import (
	"bytes"
	"fmt"
	"math/big"
	"math/rand"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rlp"
	iradix "github.com/hashicorp/go-immutable-radix"
	"github.com/stretchr/testify/assert"
)

type mockState struct {
	snapshots map[common.Hash]Snapshot
}

func (m *mockState) NewSnapshotAt(root common.Hash) (Snapshot, error) {
	t, ok := m.snapshots[root]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return t, nil
}

func (m *mockState) NewSnapshot() Snapshot {
	return &mockSnapshot{data: map[string][]byte{}}
}

func (m *mockState) GetCode(hash common.Hash) ([]byte, bool) {
	panic("Not implemented in tests")
}

type mockSnapshot struct {
	data map[string][]byte
}

func (m *mockSnapshot) Get(k []byte) ([]byte, bool) {
	v, ok := m.data[hexutil.Encode(k)]
	return v, ok
}

func (m *mockSnapshot) Commit(x *iradix.Tree) (Snapshot, []byte) {
	panic("Not implemented in tests")
}

func newStateWithPreState(preState map[common.Address]*PreState) (*mockState, *mockSnapshot) {
	state := &mockState{
		snapshots: map[common.Hash]Snapshot{},
	}
	snapshot := &mockSnapshot{
		data: map[string][]byte{},
	}
	for addr, p := range preState {
		account, snap := buildMockPreState(p)
		if snap != nil {
			state.snapshots[account.Root] = snap
		}

		accountRlp, err := rlp.EncodeToBytes(account)
		if err != nil {
			panic(err)
		}
		snapshot.data[hexutil.Encode(hashit(addr.Bytes()))] = accountRlp
	}

	return state, snapshot
}

func newTestTxn(p map[common.Address]*PreState) *Txn {
	return newTxn(newStateWithPreState(p))
}

func buildMockPreState(p *PreState) (*Account, *mockSnapshot) {
	var snap *mockSnapshot
	root := emptyStateHash

	if p.State != nil {
		data := map[string][]byte{}
		for k, v := range p.State {
			vv, _ := rlp.EncodeToBytes(bytes.TrimLeft(v.Bytes(), "\x00"))
			data[k.String()] = vv
		}
		root = randomHash()
		snap = &mockSnapshot{
			data: data,
		}
	}

	account := &Account{
		Nonce:   p.Nonce,
		Balance: big.NewInt(int64(p.Balance)),
		Root:    root,
	}
	return account, snap
}

const letterBytes = "0123456789ABCDEF"

func randomHash() common.Hash {
	b := make([]byte, common.HashLength)
	for i := range b {
		b[i] = letterBytes[rand.Intn(len(letterBytes))]
	}
	return common.BytesToHash(b)
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
