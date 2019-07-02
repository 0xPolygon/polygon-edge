package derivesha

import (
	"bytes"

	"github.com/umbracle/minimal/rlp"
	"github.com/umbracle/minimal/rlpv2"
	itrie "github.com/umbracle/minimal/state/immutable-trie"
	"github.com/umbracle/minimal/types"
)

var (
	// EmptyUncleHash is the root when there are no uncles
	EmptyUncleHash = types.StringToHash("0x1dcc4de8dec75d7aab85b567b6ccd41ad312451b948a7413f0a142fd40d49347")
)

var receiptArenaPool rlpv2.ArenaPool

func CalcReceiptRoot(receipts []*types.Receipt) types.Hash {
	keybuf := new(bytes.Buffer)

	t := itrie.NewTrie()
	txn := t.Txn()

	for indx, receipt := range receipts {
		keybuf.Reset()
		rlp.Encode(keybuf, uint(indx))

		enc := receipt.Marshal()
		txn.Insert(keybuf.Bytes(), enc)
	}

	root, _ := txn.Hash()
	return types.BytesToHash(root)
}

var txArenaPool rlpv2.ArenaPool

func CalcTxsRoot(transactions []*types.Transaction) types.Hash {
	arena := txArenaPool.Get()
	defer txArenaPool.Put(arena)

	keybuf := new(bytes.Buffer)

	t := itrie.NewTrie()
	txn := t.Txn()

	for indx, transaction := range transactions {
		keybuf.Reset()
		rlp.Encode(keybuf, uint(indx))

		v := transaction.MarshalWith(arena)
		enc := v.MarshalTo(nil)

		txn.Insert(keybuf.Bytes(), enc)
	}

	root, _ := txn.Hash()
	return types.BytesToHash(root)
}

var uncleArenaPool rlpv2.ArenaPool

func CalcUncleRoot(uncles []*types.Header) types.Hash {
	if len(uncles) == 0 {
		return EmptyUncleHash
	}

	a := uncleArenaPool.Get()
	defer uncleArenaPool.Put(a)

	v := a.NewArray()
	for _, i := range uncles {
		v.Set(i.MarshalWith(a))
	}

	return types.BytesToHash(a.HashTo(nil, v))
}
