package derivesha

import (
	"bytes"

	"github.com/umbracle/minimal/rlp"
	itrie "github.com/umbracle/minimal/state/immutable-trie"
	"github.com/umbracle/minimal/types"
	"golang.org/x/crypto/sha3"
)

func rlpHash(x interface{}) (h types.Hash) {
	hw := sha3.NewLegacyKeccak256()
	err := rlp.Encode(hw, x)
	if err != nil {
		panic(err)
	}
	hw.Sum(h[:0])
	return h
}

func CalcReceiptRoot(receipts []*types.Receipt) types.Hash {
	keybuf := new(bytes.Buffer)

	t := itrie.NewTrie()
	txn := t.Txn()

	for indx, receipt := range receipts {
		keybuf.Reset()
		rlp.Encode(keybuf, uint(indx))

		enc, _ := receipt.ConsensusEncode()
		txn.Insert(keybuf.Bytes(), enc)
	}

	root := txn.Hash(nil)
	return types.BytesToHash(root)
}

func CalcTxsRoot(transactions []*types.Transaction) types.Hash {
	keybuf := new(bytes.Buffer)

	t := itrie.NewTrie()
	txn := t.Txn()

	for indx, transaction := range transactions {
		keybuf.Reset()
		rlp.Encode(keybuf, uint(indx))

		enc, _ := rlp.EncodeToBytes(transaction)
		txn.Insert(keybuf.Bytes(), enc)
	}

	root := txn.Hash(nil)
	return types.BytesToHash(root)
}

func CalcUncleRoot(uncles []*types.Header) types.Hash {
	return rlpHash(uncles)
}
