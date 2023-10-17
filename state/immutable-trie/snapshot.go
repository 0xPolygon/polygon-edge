package itrie

import (
	"bytes"
	"fmt"

	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/fastrlp"
)

type Snapshot struct {
	state *State
	trie  *Trie
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

	key := crypto.Keccak256(rawkey.Bytes())

	val, ok := trie.Get(key, s.state.storage)
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

	data, ok := s.trie.Get(key, s.state.storage)
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

func (s *Snapshot) Commit(objs []*state.Object) (state.Snapshot, []byte, error) {
	batch := s.state.storage.Batch()

	tt := s.trie.Txn(s.state.storage)
	tt.batch = batch

	arena := stateArenaPool.Get()
	defer stateArenaPool.Put(arena)

	for _, obj := range objs {
		if obj.Deleted {
			tt.Delete(hashit(obj.Address.Bytes()))
		} else {
			account := state.Account{
				Balance:  obj.Balance,
				Nonce:    obj.Nonce,
				CodeHash: obj.CodeHash.Bytes(),
				Root:     obj.Root, // old root
			}

			if len(obj.Storage) != 0 {
				trie, err := s.state.newTrieAt(obj.Root)
				if err != nil {
					return nil, types.ZeroHash[:], fmt.Errorf("snapshot commit failed to create trie: %w", err)
				}

				localTxn := trie.Txn(s.state.storage)
				localTxn.batch = batch

				for _, entry := range obj.Storage {
					k := hashit(entry.Key)
					if entry.Deleted {
						localTxn.Delete(k)
					} else {
						vv := arena.NewBytes(bytes.TrimLeft(entry.Val, "\x00"))
						localTxn.Insert(k, vv.MarshalTo(nil))
					}
				}

				accountStateRoot, _ := localTxn.Hash()
				accountStateTrie := localTxn.Commit()

				// Add this to the cache
				s.state.AddState(types.BytesToHash(accountStateRoot), accountStateTrie)

				account.Root = types.BytesToHash(accountStateRoot)
			}

			if obj.DirtyCode {
				batch.Put(GetCodeKey(obj.CodeHash), obj.Code)
			}

			vv := account.MarshalWith(arena)
			data := vv.MarshalTo(nil)

			tt.Insert(hashit(obj.Address.Bytes()), data)
			arena.Reset()
		}
	}

	root, err := tt.Hash()
	if err != nil {
		return nil, types.ZeroHash[:], fmt.Errorf("snapshot commit can not retrieve hash: %w", err)
	}

	nTrie := tt.Commit()

	// Write all the entries to db
	if err := batch.Write(); err != nil {
		return nil, types.ZeroHash[:], fmt.Errorf("snapshot commit db write error: %w", err)
	}

	s.state.AddState(types.BytesToHash(root), nTrie)

	return &Snapshot{trie: nTrie, state: s.state}, root, nil
}
