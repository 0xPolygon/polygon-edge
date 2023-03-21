package itrie

import (
	"testing"

	"github.com/syndtr/goleveldb/leveldb"
	ldbstorage "github.com/syndtr/goleveldb/leveldb/storage"
	"pgregory.net/rapid"
)

func TestCompareModelOfTrieCopy(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(tt *rapid.T) {
		ldbStorageOld := ldbstorage.NewMemStorage()
		ldbStorageNew := ldbstorage.NewMemStorage()
		ldb, err := leveldb.Open(ldbStorageOld, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer ldb.Close()

		ldbNew, err := leveldb.Open(ldbStorageNew, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer ldbNew.Close()

		kv := NewKV(ldb)
		newKV := NewKV(ldbNew)
		state := NewState(kv)
		trie := state.newTrie()
		tx := trie.Txn(kv)

		n := rapid.IntRange(1, 1000).Draw(tt, "n")
		for i := 0; i < n; i++ {
			key := rapid.SliceOfN(rapid.Byte(), 32, 32).Draw(tt, "key")
			value := rapid.SliceOfN(rapid.Byte(), 10, 80).Draw(tt, "value")
			tx.Insert(key, value)
		}

		tx.Commit()
		stateRoot := trie.Hash()
		result, err := HashChecker(stateRoot.Bytes(), kv)
		if err != nil {
			t.Fatal(err)
		}
		if stateRoot != result {
			t.Fatal("Hashes are not equal", stateRoot, result)
		}

		err = CopyTrie(stateRoot.Bytes(), kv, newKV, []byte{}, false)
		if err != nil {
			t.Fatal(err)
		}

		result, err = HashChecker(stateRoot.Bytes(), newKV)
		if err != nil {
			t.Error(err)
		}
		if stateRoot != result {
			t.Error("Hashes are not equal", stateRoot, result)
		}
	})
}
