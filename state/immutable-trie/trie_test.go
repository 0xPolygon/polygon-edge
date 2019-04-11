package trie

import (
	"fmt"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestSimple(t *testing.T) {

	ss := NewMemoryStorage()
	tt := NewTrie()
	txn := tt.Txn()

	txn.Insert([]byte("hello"), []byte("hello"))
	txn.Insert([]byte("world"), []byte("world"))

	root := txn.Hash(ss)

	tt2 := txn.Commit()
	tt2.root.Show()

	// Load the new trie

	tt3, err := NewTrieAt(ss, common.BytesToHash(root))
	if err != nil {
		t.Fatal(err)
	}

	tt3.root.Show()
	root2 := tt3.Root().Hash(ss)

	fmt.Println(root)
	fmt.Println(root2)
}
