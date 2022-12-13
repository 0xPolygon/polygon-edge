package itrie

import (
	"encoding/hex"

	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/fastrlp"
)

type Snapshot struct {
	state  *State
	trie   *Trie
	tracer *tracer
}

var emptyStateHash = types.StringToHash("0x56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421")

func (s *Snapshot) GetStorage(addr types.Address, root types.Hash, rawkey types.Hash) types.Hash {
	var (
		err  error
		trie *Trie
	)

	if root == emptyStateHash {
		trie = s.state.newTrie()
	} else {
		trie, err = s.state.newTrieAt(root)
		if err != nil {
			return types.Hash{}
		}
	}

	trie.tracer = s.tracer

	key := crypto.Keccak256(rawkey.Bytes())

	val, ok := trie.Get(key)
	if !ok {
		return types.Hash{}
	}

	p := &fastrlp.Parser{}

	v, err := p.Parse(val)
	if err != nil {
		return types.Hash{}
	}

	res := []byte{}
	if res, err = v.GetBytes(res[:0]); err != nil {
		return types.Hash{}
	}

	return types.BytesToHash(res)
}

func (s *Snapshot) GetAccount(addr types.Address) (*state.Account, error) {
	key := crypto.Keccak256(addr.Bytes())
	s.trie.tracer = s.tracer

	data, ok := s.trie.Get(key)
	if !ok {
		return nil, nil
	}

	var account state.Account
	if err := account.UnmarshalRlp(data); err != nil {
		return nil, err
	}

	return &account, nil
}

func (s *Snapshot) GetCode(hash types.Hash) ([]byte, bool) {
	return s.state.GetCode(hash)
}

func (s *Snapshot) Commit(objs []*state.Object) (state.Snapshot, *types.Trace, []byte) {
	trie, root := s.trie.Commit(objs)

	s.tracer.Proof()

	return &Snapshot{trie: trie, state: s.state}, &types.Trace{Trace: s.tracer.Traces()}, root
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
		if i != nil {
			hasher.hashImpl(i, arena, false, 0)
		}
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
