package itrie

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
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
	tr *tracer
}

func NewTraceStore(tr *tracer) Storage {
	return &traceStore{
		tr: tr,
	}
}

func (ts *traceStore) Get(k []byte) ([]byte, bool) {
	var v string
	if ts.tr.isAccountTrie {
		v = ts.tr.trace.AccountTrie[hex.EncodeToString(k)]
	} else {
		v = ts.tr.trace.StorageTrie[hex.EncodeToString(k)]
		// fmt.Printf("key: %s, value: %s\n", hex.EncodeToString(k), v)
	}
	val, _ := hex.DecodeString(v)

	if len(val) == 0 {
		fmt.Printf("**** value not found for key: %s\n", hex.EncodeToString(k))
		return nil, false
	}

	return val, true
}

func (traceStore) Put(k, v []byte)                        {}
func (traceStore) Batch() Batch                           { return nil }
func (traceStore) SetCode(hash types.Hash, code []byte)   {}
func (traceStore) GetCode(hash types.Hash) ([]byte, bool) { return nil, false }
func (traceStore) Close() error                           { return nil }

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
	//tt := NewTrie()

	ltr, err := LoadTrace()
	require.NoError(t, err)

	accountTracer := &tracer{
		isAccountTrie: true,
		trace:         ltr,
	}
	ts := NewTraceStore(accountTracer)
	s := NewState(ts)
	sn, err := s.NewSnapshotAt(ltr.ParentStateRoot)
	require.NoError(t, err)

	addr := types.StringToAddress("0x6FdA56C57B0Acadb96Ed5624aC500C0429d59429")
	acc, err := sn.GetAccount(addr)
	require.NoError(t, err)

	storageTracer := &tracer{
		isAccountTrie: false,
		trace:         ltr,
	}
	ts = NewTraceStore(storageTracer)
	s = NewState(ts)
	tt, err := s.newTrieAt(acc.Root)
	require.NoError(t, err)

	// Load the trie from the trace.
	txn := tt.Txn(ts)

	// txn.Insert(types.StringToBytes("0x0000000000000000000000000000000000000000000000000000000000000002"), types.StringToBytes("0x00000000000000000000000000000000000000000000000000000000000048d1"))

	b := txn.Lookup(types.StringToBytes("0x04e2f49b914b2b5972f4e339e6dc75455852ed61bac5676f58a2b7036def7b18"))
	t.Logf("value: %s\n", hex.EncodeToString(b))

	for _, txt := range ltr.TxnTraces {
		je := txt.Delta[addr]
		for slot, _ := range je.StorageRead {
			v := txn.Lookup(slot.Bytes())
			if v == nil {
				t.Logf("slot %s not found", slot)
			}

			// assert.Equal(t, val.Bytes(), v)
		}
	}
}
