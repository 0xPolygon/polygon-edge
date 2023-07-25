package itrie

import (
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/syndtr/goleveldb/leveldb"
	ldbstorage "github.com/syndtr/goleveldb/leveldb/storage"
	"github.com/umbracle/fastrlp"
)

func TestTrie_Proof(t *testing.T) {
	acct := state.Account{
		Balance: big.NewInt(10),
	}
	val := acct.MarshalWith(&fastrlp.Arena{}).MarshalTo(nil)

	acct2 := state.Account{
		Balance: big.NewInt(20),
	}
	val2 := acct2.MarshalWith(&fastrlp.Arena{}).MarshalTo(nil)

	tt := NewTrie()

	ldbStorage := ldbstorage.NewMemStorage()
	ldb, err := leveldb.Open(ldbStorage, nil)
	require.NoError(t, err)

	defer ldb.Close()

	kv := NewKV(ldb)

	txn := tt.Txn(kv)

	tr := &tracer{
		isAccountTrie: true,
		trace:         &types.Trace{},
	}
	txn.tracer = tr

	_, err = txn.Hash()
	require.NoError(t, err)

	txn.Insert([]byte{0x1, 0x2}, val)
	txn.Insert([]byte{0x1, 0x3}, val2)
	txn.Lookup([]byte{0x1, 0x2})

	txn.Delete([]byte{0x1, 0x3})

	assert.IsType(t, &ShortNode{}, tr.nodes[0])
	assert.IsType(t, &FullNode{}, tr.nodes[1])
	assert.IsType(t, &ShortNode{}, tr.nodes[2])
	assert.IsType(t, &ValueNode{}, tr.nodes[3])
	_, h := tr.nodes[1].Hash()
	assert.False(t, h)

	n := tr.nodes[4].(*ShortNode)
	v, _ := n.child.Hash()
	assert.Equal(t, val, v)

	tr.Proof()

	for k, v := range tr.trace.AccountTrie {
		t.Logf("key: %s, value: %s\n", k, v)
	}
}
