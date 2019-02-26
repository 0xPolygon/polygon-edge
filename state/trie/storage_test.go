package trie

import (
	"fmt"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestStorage(t *testing.T) {

	tt := NewTrie()
	txn := tt.Txn()
	txn.Insert([]byte{0x2}, []byte{0x1})
	txn.Insert([]byte{0x1}, []byte{0x2})
	txn.Insert([]byte{0x1, 0x2, 0x3, 0x4}, []byte{0x3})

	ttAux := txn.Commit()

	ttAux.Root().Show()

	s := NewMemoryStorage()
	root := txn.Hash(s)

	ttt, err := NewTrieAt(s, common.BytesToHash(root))
	if err != nil {
		t.Fatal(err)
	}

	ttt.Root().Show()

	fmt.Println(ttt.Root().Equal(ttAux.Root()))
}
