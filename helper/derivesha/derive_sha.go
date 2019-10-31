package derivesha

import (
	"github.com/umbracle/fastrlp"
	"github.com/umbracle/minimal/helper/keccak"
	itrie "github.com/umbracle/minimal/state/immutable-trie"
	"github.com/umbracle/minimal/types"
)

var receiptArenaPool fastrlp.ArenaPool

func CalcReceiptRoot(receipts []*types.Receipt) types.Hash {
	ar := receiptArenaPool.Get()
	defer receiptArenaPool.Put(ar)

	t := itrie.NewTrie()
	txn := t.Txn()

	for indx, receipt := range receipts {
		v0 := ar.NewUint(uint64(indx))
		v1 := receipt.MarshalWith(ar)

		txn.Insert(v0.MarshalTo(nil), v1.MarshalTo(nil))
	}

	root, _ := txn.Hash()
	return types.BytesToHash(root)
}

var txArenaPool fastrlp.ArenaPool

func CalcTxsRoot(transactions []*types.Transaction) types.Hash {
	ar := txArenaPool.Get()
	defer txArenaPool.Put(ar)

	t := itrie.NewTrie()
	txn := t.Txn()

	for indx, transaction := range transactions {
		v0 := ar.NewUint(uint64(indx))
		v1 := transaction.MarshalWith(ar)

		txn.Insert(v0.MarshalTo(nil), v1.MarshalTo(nil))
	}

	root, _ := txn.Hash()
	return types.BytesToHash(root)
}

var uncleArenaPool fastrlp.ArenaPool

func CalcUncleRoot(uncles []*types.Header) types.Hash {
	if len(uncles) == 0 {
		return types.EmptyUncleHash
	}

	a := uncleArenaPool.Get()
	v := a.NewArray()
	for _, i := range uncles {
		v.Set(i.MarshalWith(a))
	}

	root := keccak.Keccak256Rlp(nil, v)
	uncleArenaPool.Put(a)

	return types.BytesToHash(root)
}
