package itrie

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/state"
	"github.com/umbracle/fastrlp"
)

func TestTrie_Proof(t *testing.T) {
	acct := state.Account{
		Balance: big.NewInt(10),
	}
	val := acct.MarshalWith(&fastrlp.Arena{}).MarshalTo(nil)

	tt := NewTrie()
	txn := tt.Txn()
	txn.Insert([]byte{0x1, 0x2}, val)
	txn.Insert([]byte{0x1, 0x1}, val)

	tracer := &tracer{}
	txn.trace = tracer

	txn.Hash()
	txn.Lookup([]byte{0x1, 0x2})

	tracer.Proof()
	fmt.Println(tracer.Traces())
}

type tracer struct {
	nodes []Node

	traces map[string]string
}

func (t *tracer) Traces() map[string]string {
	return t.traces
}

func (t *tracer) Trace(n Node) {
	if t.nodes == nil {
		t.nodes = []Node{}
	}

	t.nodes = append(t.nodes, n)
}

func (t *tracer) Proof() {
	hasher, _ := hasherPool.Get().(*hasher)
	defer hasherPool.Put(hasher)

	hasher.WithBatch(t)

	arena, _ := hasher.AcquireArena()
	defer hasher.ReleaseArenas(0)

	for _, i := range t.nodes {
		hasher.hashImpl(i, arena, false, 0)
	}
}

func (t *tracer) Put(k, v []byte) {
	if t.traces == nil {
		t.traces = map[string]string{}
	}

	kStr := hex.EncodeToString(k)
	if _, ok := t.traces[kStr]; !ok {
		t.traces[kStr] = hex.EncodeToString(v)
	}
}
