package itrie

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/0xPolygon/polygon-edge/crypto"

	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/umbracle/fastrlp"
)

var emptyCodeHash = crypto.Keccak256(nil)

func getCustomNode(hash []byte, storage Storage) (Node, []byte, error) {
	data, ok, err := storage.Get(hash)
	if err != nil || !ok {
		return nil, nil, err
	}

	// NOTE. We dont need to make copies of the bytes because the nodes
	// take the reference from data itself which is a safe copy.
	p := parserPool.Get()
	defer parserPool.Put(p)

	v, err := p.Parse(data)
	if err != nil {
		return nil, nil, err
	}

	if v.Type() != fastrlp.TypeArray {
		return nil, nil, fmt.Errorf("storage item should be an array")
	}

	n, err := decodeNode(v, storage)

	return n, data, err
}

func CopyTrie(nodeHash []byte, storage Storage, newStorage Storage, agg []byte, isStorage bool) error {
	batchWriter := newStorage.Batch()

	if err := copyTrieHash(nodeHash, storage, batchWriter, agg, isStorage); err != nil {
		return err
	}

	return batchWriter.Write()
}

func copyTrieHash(nodeHash []byte, storage Storage, batchWriter Batch, agg []byte, isStorage bool) error {
	node, data, err := getCustomNode(nodeHash, storage)
	if err != nil {
		return err
	}

	//copy whole bytes of nodes
	batchWriter.Put(nodeHash, data)

	return copyTrieNode(node, storage, batchWriter, agg, isStorage)
}

func copyTrieNode(node Node, storage Storage, batchWriter Batch, agg []byte, isStorage bool) error {
	switch n := node.(type) {
	case nil:
		return nil
	case *FullNode:
		if len(n.hash) > 0 {
			return copyTrieHash(n.hash, storage, batchWriter, agg, isStorage)
		}

		for i := range n.children {
			if n.children[i] == nil {
				continue
			}

			err := copyTrieNode(n.children[i], storage, batchWriter, append(agg, uint8(i)), isStorage)
			if err != nil {
				return err
			}
		}

	case *ValueNode:
		//if node represens stored value, then we need to copy it
		if n.hash {
			return copyTrieHash(n.buf, storage, batchWriter, agg, isStorage)
		}

		if !isStorage {
			var account state.Account
			if err := account.UnmarshalRlp(n.buf); err != nil {
				return fmt.Errorf("can't parse account %s: %w", hex.EncodeToString(encodeCompact(agg)), err)
			} else {
				if account.CodeHash != nil && bytes.Equal(account.CodeHash, emptyCodeHash) == false {
					hash := types.BytesToHash(account.CodeHash)

					code, ok := storage.GetCode(hash)
					if ok {
						batchWriter.Put(GetCodeKey(hash), code)
					} else {
						return fmt.Errorf("can't find code %s", hex.EncodeToString(account.CodeHash))
					}
				}

				if account.Root != types.EmptyRootHash {
					return copyTrieHash(account.Root[:], storage, batchWriter, nil, true)
				}
			}
		}

	case *ShortNode:
		if len(n.hash) > 0 {
			return copyTrieHash(n.hash, storage, batchWriter, agg, isStorage)
		}

		return copyTrieNode(n.child, storage, batchWriter, append(agg, n.key...), isStorage)
	}

	return nil
}

func HashChecker(stateRoot []byte, storage Storage) (types.Hash, error) {
	node, _, err := GetNode(stateRoot, storage)
	if err != nil {
		return types.Hash{}, err
	}

	h, ok := hasherPool.Get().(*hasher)
	if !ok {
		return types.Hash{}, errors.New("can't get hasher")
	}

	arena, _ := h.AcquireArena()

	val, err := hashChecker(node, h, arena, 0, storage)
	if err != nil {
		return types.Hash{}, err
	}

	if val == nil {
		return emptyStateHash, nil
	}

	h.ReleaseArenas(0)
	hasherPool.Put(h)

	return types.BytesToHash(val.Raw()), nil
}

func hashChecker(node Node, h *hasher, a *fastrlp.Arena, d int, storage Storage) (*fastrlp.Value, error) {
	var (
		val *fastrlp.Value
		aa  *fastrlp.Arena
		idx int
	)

	switch n := node.(type) {
	case nil:
		return nil, nil
	case *ValueNode:
		if n.hash {
			nd, _, err := GetNode(n.buf, storage)
			if err != nil {
				return nil, err
			}

			return hashChecker(nd, h, a, d, storage)
		}

		return a.NewCopyBytes(n.buf), nil

	case *ShortNode:
		child, err := hashChecker(n.child, h, a, d+1, storage)
		if err != nil {
			return nil, err
		}

		val = a.NewArray()
		val.Set(a.NewBytes(encodeCompact(n.key)))
		val.Set(child)

	case *FullNode:
		val = a.NewArray()

		aa, idx = h.AcquireArena()

		for _, i := range n.children {
			if i == nil {
				val.Set(a.NewNull())
			} else {
				v, err := hashChecker(i, h, aa, d+1, storage)
				if err != nil {
					return nil, err
				}
				val.Set(v)
			}
		}

		// Add the value
		if n.value == nil {
			val.Set(a.NewNull())
		} else {
			v, err := hashChecker(n.value, h, a, d+1, storage)
			if err != nil {
				return nil, err
			}
			val.Set(v)
		}

	default:
		return nil, fmt.Errorf("unknown node type %T", node)
	}

	if val.Len() < 32 {
		return val, nil
	}

	// marshal RLP value
	h.buf = val.MarshalTo(h.buf[:0])

	if aa != nil {
		h.ReleaseArenas(idx)
	}

	tmp := h.Hash(h.buf)
	hh := node.SetHash(tmp)

	return a.NewCopyBytes(hh), nil
}

func NewKV(db *leveldb.DB) *KVStorage {
	return &KVStorage{db: db}
}

func NewTrieWithRoot(root Node) *Trie {
	return &Trie{
		root: root,
	}
}
