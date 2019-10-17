package derivesha

import (
	"github.com/umbracle/fastrlp"
	"github.com/umbracle/minimal/helper/keccak"
	itrie "github.com/umbracle/minimal/state/immutable-trie"
	"github.com/umbracle/minimal/types"
)

var (
	// EmptyUncleHash is the root when there are no uncles
	EmptyUncleHash = types.StringToHash("0x1dcc4de8dec75d7aab85b567b6ccd41ad312451b948a7413f0a142fd40d49347")
)

var receiptArenaPool fastrlp.ArenaPool

func CalcReceiptRoot(receipts []*types.Receipt) types.Hash {
	// keybuf := new(bytes.Buffer)

	ar := receiptArenaPool.Get()
	defer receiptArenaPool.Put(ar)

	t := itrie.NewTrie()
	txn := t.Txn()

	for indx, receipt := range receipts {
		v0 := ar.NewUint(uint64(indx))
		v1 := receipt.MarshalWith(ar)

		//enc := receipt.MarshalWith()
		//txn.Insert(keybuf.Bytes(), enc)
		txn.Insert(v0.MarshalTo(nil), v1.MarshalTo(nil))
	}

	root, _ := txn.Hash()
	return types.BytesToHash(root)
}

var txArenaPool fastrlp.ArenaPool

func CalcTxsRoot(transactions []*types.Transaction) types.Hash {
	// fmt.Println("-- calc txn root --")

	ar := txArenaPool.Get()
	defer txArenaPool.Put(ar)

	// keybuf := new(bytes.Buffer)

	t := itrie.NewTrie()
	txn := t.Txn()

	for indx, transaction := range transactions {
		// keybuf.Reset()
		// rlp.Encode(keybuf, uint(indx))
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
		return EmptyUncleHash
	}

	hash := keccak.DefaultKeccakPool.Get()
	a := uncleArenaPool.Get()

	v := a.NewArray()
	for _, i := range uncles {
		v.Set(i.MarshalWith(a))
	}

	dst := hash.WriteRlp(nil, v)

	keccak.DefaultKeccakPool.Put(hash)
	uncleArenaPool.Put(a)

	return types.BytesToHash(dst)
}
