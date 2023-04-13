package itrie

import (
	"encoding/hex"

	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/fastrlp"
)

type Snapshot struct {
	state *State
	trie  *Trie

	// tracers is the list of tracers executed, one for each trie
	tracers []*tracer

	// trace is the object that holds the traces
	trace *types.Trace
}

var emptyStateHash = types.StringToHash("0x56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421")

func (s *Snapshot) createTracer(isAccountTrie bool) *tracer {
	if s.tracers == nil {
		s.tracers = []*tracer{}
	}

	tr := &tracer{isAccountTrie: isAccountTrie, trace: s.trace}
	s.tracers = append(s.tracers, tr)

	return tr
}

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

	trie.tracer = s.createTracer(false)

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
	s.trie.tracer = s.createTracer(true)

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

	for _, t := range s.tracers {
		t.Proof()
	}

	return &Snapshot{trie: trie, state: s.state, trace: &types.Trace{}}, s.trace, root
}

type tracer struct {
	// isAccountTrie represents whether the trace is in an
	// acount or storage trie
	isAccountTrie bool

	// nodes is the list of nodes from the partial merkle trie
	// accessed during the trace
	nodes []Node

	// trace is the reference object to dump the traces
	trace *types.Trace
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
	kStr := hex.EncodeToString(k)

	if t.isAccountTrie {
		if t.trace.AccountTrie == nil {
			t.trace.AccountTrie = map[string]string{}
		}

		if _, ok := t.trace.AccountTrie[kStr]; !ok {
			t.trace.AccountTrie[kStr] = hex.EncodeToString(v)
		}
	} else {
		if t.trace.StorageTrie == nil {
			t.trace.StorageTrie = map[string]string{}
		}

		if _, ok := t.trace.StorageTrie[kStr]; !ok {
			t.trace.StorageTrie[kStr] = hex.EncodeToString(v)
		}
	}
}
