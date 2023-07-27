package itrie

import (
	"encoding/hex"
	"encoding/json"
	"io/ioutil"
	"math/big"
	"os"
	"testing"

	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/syndtr/goleveldb/leveldb"
	ldbstorage "github.com/syndtr/goleveldb/leveldb/storage"
	"github.com/umbracle/fastrlp"
)

type traceStore struct {
	// trace is the object that holds the traces
	trace *types.Trace
}

func NewTraceStore(trace *types.Trace) Storage {
	return &traceStore{
		trace: trace,
	}
}

func (traceStore) Put(k, v []byte) {
}

func (ts *traceStore) Get(k []byte) ([]byte, bool) {
	return nil, false
}

func (traceStore) Batch() Batch {
	return nil
}

func (traceStore) SetCode(hash types.Hash, code []byte) {

}

func (traceStore) GetCode(hash types.Hash) ([]byte, bool) {
	return nil, false
}

func (traceStore) Close() error {
	return nil
}

func LoadTrace() (*types.Trace, error) {
	// Load Trace structure from JSON file.
	traceFile, err := os.Open("7410_readable.json")
	if err != nil {
		return nil, err
	}
	defer traceFile.Close()

	// Read JSON file and desrialize it into Trace structure.
	// Read the whole file
	tf, err := ioutil.ReadAll(traceFile)
	if err != nil {
		return nil, err
	}

	trace := &types.Trace{}
	err = json.Unmarshal(tf, trace)
	if err != nil {
		return nil, err
	}

	return trace, nil
}

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

func TestTrie_Load(t *testing.T) {
	tt := NewTrie()

	ldbStorage := ldbstorage.NewMemStorage()
	ldb, err := leveldb.Open(ldbStorage, nil)
	require.NoError(t, err)

	defer ldb.Close()

	kv := NewKV(ldb)

	ltr, err := LoadTrace()
	require.NoError(t, err)
	// ts := NewTraceStore(ltr)

	// Load the trie from the trace.
	// txn := tt.Txn(ts)
	txn := tt.Txn(kv)
	for k, v := range ltr.StorageTrie {
		tk, _ := hex.DecodeString(k)
		tv, _ := hex.DecodeString(v)

		txn.Insert(tk, tv)
	}
	tt = txn.Commit()

	for _, txt := range ltr.TxnTraces {
		je := txt.Delta[types.StringToAddress("0x6FdA56C57B0Acadb96Ed5624aC500C0429d59429")]
		for slot, _ := range je.Storage {
			v := txn.Lookup(slot.Bytes())
			if v == nil {
				t.Logf("slot %s not found", slot)
			}
		}
	}
}
