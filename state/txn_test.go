package state

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
	"github.com/umbracle/fastrlp"
	"golang.org/x/crypto/sha3"
)

type mockState struct {
	snapshots map[types.Hash]Snapshot
}

func (m *mockState) NewSnapshotAt(root types.Hash) (Snapshot, error) {
	t, ok := m.snapshots[root]
	if !ok {
		return nil, fmt.Errorf("not found")
	}

	return t, nil
}

func (m *mockState) NewSnapshot() Snapshot {
	return &mockSnapshot{data: map[string][]byte{}}
}

func (m *mockState) GetCode(hash types.Hash) ([]byte, bool) {
	panic("Not implemented in tests")
}

type mockSnapshot struct {
	data map[string][]byte
}

func (m *mockSnapshot) Get(k []byte) ([]byte, bool) {
	v, ok := m.data[hex.EncodeToHex(k)]

	return v, ok
}

func (m *mockSnapshot) Commit(objs []*Object) (Snapshot, []byte) {
	panic("Not implemented in tests")
}

func newStateWithPreState(preState map[types.Address]*PreState) (*mockState, *mockSnapshot) {
	state := &mockState{
		snapshots: map[types.Hash]Snapshot{},
	}
	snapshot := &mockSnapshot{
		data: map[string][]byte{},
	}

	ar := &fastrlp.Arena{}

	for addr, p := range preState {
		account, snap := buildMockPreState(p)
		if snap != nil {
			state.snapshots[account.Root] = snap
		}

		v := account.MarshalWith(ar)
		accountRlp := v.MarshalTo(nil)
		/*
			accountRlp, err := rlp.EncodeToBytes(account)
			if err != nil {
				panic(err)
			}
		*/
		snapshot.data[hex.EncodeToHex(hashit(addr.Bytes()))] = accountRlp
	}

	return state, snapshot
}

func newTestTxn(p map[types.Address]*PreState) *Txn {
	return newTxn(newStateWithPreState(p))
}

func buildMockPreState(p *PreState) (*Account, *mockSnapshot) {
	var snap *mockSnapshot

	root := emptyStateHash

	ar := &fastrlp.Arena{}

	if p.State != nil {
		data := map[string][]byte{}

		for k, v := range p.State {
			vv := ar.NewBytes(bytes.TrimLeft(v.Bytes(), "\x00"))
			data[k.String()] = vv.MarshalTo(nil)
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

func randomHash() types.Hash {
	b := make([]byte, types.HashLength)
	for i := range b {
		randNum, _ := rand.Int(rand.Reader, big.NewInt(int64(len(letterBytes))))
		b[i] = letterBytes[randNum.Int64()]
	}

	return types.BytesToHash(b)
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

func hashit(k []byte) []byte {
	h := sha3.NewLegacyKeccak256()
	h.Write(k)

	return h.Sum(nil)
}
