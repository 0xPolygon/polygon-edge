package itrie

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"testing"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/state/runtime"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/syndtr/goleveldb/leveldb"
	ldbstorage "github.com/syndtr/goleveldb/leveldb/storage"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
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
		fmt.Printf("-- GET FROM TRACE -- key: %s, value: %s\n", hex.EncodeToString(k), v)
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
	traceFile, err := os.Open("71000_trace_readable.json")
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

	addr := types.StringToAddress("0x0000000000000000000000000000000000000101")
	acc, err := sn.GetAccount(addr)
	require.NotNil(t, acc)
	require.NoError(t, err)

	storageTracer := &tracer{
		isAccountTrie: false,
		trace:         ltr,
	}
	ts = NewTraceStore(storageTracer)
	s = NewState(ts)
	// tt, err := s.newTrieAt(acc.Root)
	log.Println("root: ", acc.Root)
	require.NoError(t, err)

	// Load the trie from the trace.
	// txn := tt.Txn(ts)

	// txn.Insert(types.StringToBytes("0x0000000000000000000000000000000000000000000000000000000000000002"), types.StringToBytes("0x00000000000000000000000000000000000000000000000000000000000048d1"))

	traceSnap := NewTraceStoreTxn(t, ltr, acc.Root)
	b := traceSnap.GetStorage(addr, acc.Root, types.StringToHash("0xb7d815cabb43222c333e6792c1a90fe7f30d238ce576408088a4ff29c49efc73"))
	// b := txn.Lookup(types.StringToBytes("0x2c64b4c28102eb31817db0aae9385bd83769912689d15cb6b0f59dd7eff20613"))
	// t.Logf("value: %s\n", hex.EncodeToString(b))
	t.Logf("-- VALUE --: %s\n", b.String())

	// for _, txt := range ltr.TxnTraces {
	// 	je := txt.Delta[addr]
	// 	for slot, _ := range je.StorageRead {
	// 		v := txn.Lookup(slot.Bytes())
	// 		if v == nil {
	// 			t.Logf("slot %s not found", slot)
	// 		}

	// 		// assert.Equal(t, val.Bytes(), v)
	// 	}
	// }
}

func TestTrie_RandomOps(t *testing.T) {
	addr1 := types.StringToAddress("1")
	// addr2 := types.StringToAddress("2")

	s := NewState(NewMemoryStorage())
	snap := s.NewSnapshot()

	// Check if the generated trace includes all nodes
	// radix.SetBalance(addr1, big.NewInt(1))
	// radix.SetNonce(addr1, 1)
	// radix.SetNonce(addr1, 2) // updates

	oneHash := types.Hash{0x1}
	twoHash := types.Hash{0xf}
	threeHash := types.Hash{0x3}

	// radix.SetState(addr1, oneHash, twoHash)
	// _ = radix.GetState(addr1, types.ZeroHash)
	// radix.SetState(addr1, types.ZeroHash, oneHash) // updates
	// _ = radix.GetState(addr1, types.ZeroHash)

	// _ = radix.GetState(addr1, twoHash)
	// _ = radix.GetState(addr1, threeHash)

	// radix.SetState(addr1, oneHash, oneHash)

	// _ = radix.GetCode(addr1)

	// radix.TouchAccount(addr1)

	radix := state.NewTxn(snap)
	radix.SetState(addr1, oneHash, twoHash)
	radix.SetState(addr1, twoHash, threeHash)
	objs, err := radix.Commit(false)
	require.NoError(t, err)
	snap, tr, _ := snap.Commit(objs)

	radix = state.NewTxn(snap)
	val1 := radix.GetState(addr1, oneHash)
	assert.Equal(t, val1, twoHash)
	val2 := radix.GetState(addr1, twoHash)
	assert.Equal(t, val2, threeHash)

	// Perform some operations
	// _ = radix.GetState(addr1, oneHash)
	// radix.SetBalance(addr1, big.NewInt(2)) // updates

	// _ = radix.GetState(addr1, twoHash)
	// _ = radix.GetState(addr1, threeHash)

	// radix.SetState(addr1, oneHash, twoHash)

	objs, err = radix.Commit(false)
	require.NoError(t, err)

	// Get the trace and add the journal for this transaction
	_, tr, _ = snap.Commit(objs)
	journal := radix.GetCompactJournal()
	tr.TxnTraces = []*types.TxnTrace{
		{
			Delta: journal,
		},
	}

	out, _ := json.MarshalIndent(tr, "", "  ")
	log.Println(string(out))

	acc1, _ := radix.GetAccount(addr1)
	traceSnap := NewTraceStoreTxn(t, tr, acc1.Root)

	// Loop through the StorageRead for addr1
	for sk := range journal[addr1].StorageRead {
		// Check if the entry is in the StorageTrie trace
		val := traceSnap.GetStorage(addr1, acc1.Root, sk)
		// val := txn.Lookup(sk.Bytes())
		t.Log("-- THE VAL --", val.String())
		// assert.NotNil(t, val)
	}
}

func NewTraceStoreTxn(t *testing.T, trace *types.Trace, root types.Hash) state.Snapshot {
	storageTracer := &tracer{
		isAccountTrie: false,
		trace:         trace,
	}
	ts := NewTraceStore(storageTracer)
	s := NewState(ts)
	return s.NewSnapshot()

	// Load the trie from the trace.
	// return tt.Txn(ts)
}

func Test_Transition(t *testing.T) {
	senderAddr := types.Address{1} // account that sends transactions
	alloc := map[types.Address]*chain.GenesisAccount{
		senderAddr: {Balance: ethgo.Ether(100)}, // give some ethers to sender
	}
	transition := newTestTransition(t, alloc, false)
	code := contractsapi.TestSimple.Bytecode
	// deploy contracts
	contractAddr := transitionDeployContract(t, transition, []byte(code), senderAddr)
	snap, tr, _, err := transition.Commit()
	require.NoError(t, err)
	log.Println(tr)

	forks := chain.ForksInTime{}
	forks.London = false
	forks.Constantinople = true
	transition = state.NewTransition(forks, snap, state.NewTxn(snap))
	setValueFn := abi.MustNewMethod("function setValue(uint256 _val) public")
	newVal := big.NewInt(1)
	input, err := setValueFn.Encode([]interface{}{newVal})
	require.NoError(t, err)
	transitionCallContract(t, transition, contractAddr, senderAddr, input)
	snap, tr, _, err = transition.Commit()

	out, _ := json.MarshalIndent(tr, "", "  ")
	log.Println(string(out))

	//MissingSlot(trace, nil, t)
	newVal = big.NewInt(2)
	input, err = setValueFn.Encode([]interface{}{newVal})
	require.NoError(t, err)
	transition = state.NewTransition(forks, snap, state.NewTxn(snap))
	transitionCallContract(t, transition, contractAddr, senderAddr, input)
	_, tr, _, err = transition.Commit()
	// MissingSlot(trace, nil, t)

	out, _ = json.MarshalIndent(tr, "", "  ")
	log.Println(string(out))
}

func transitionDeployContract(t *testing.T, transition *state.Transition, byteCode []byte,
	sender types.Address) types.Address {
	deployResult := transition.Create2(sender, byteCode, big.NewInt(0), 1e9)
	require.NoError(t, deployResult.Err)
	return deployResult.Address
}

func transitionCallContract(t *testing.T, transition *state.Transition, contractAddress types.Address,
	sender types.Address, input []byte) *runtime.ExecutionResult {
	t.Helper()
	result := transition.Call2(sender, contractAddress, input, big.NewInt(0), math.MaxUint64)
	require.NoError(t, result.Err)
	return result
}
func newTestTransition(t *testing.T, alloc map[types.Address]*chain.GenesisAccount, disk bool) *state.Transition {
	t.Helper()
	var st *State
	if disk {
		stateStorage, err := NewLevelDBStorage(filepath.Join(t.TempDir(), "trie"), hclog.NewNullLogger())
		require.NoError(t, err)
		st = NewState(stateStorage)
	} else {
		st = NewState(NewMemoryStorage())
	}
	forks := chain.AllForksEnabled
	forks.RemoveFork("london")
	ex := state.NewExecutor(&chain.Params{
		Forks: forks,
	}, st, hclog.NewNullLogger())
	rootHash, err := ex.WriteGenesis(alloc, types.Hash{})
	require.NoError(t, err)
	ex.GetHash = func(h *types.Header) state.GetHashByNumber {
		return func(i uint64) types.Hash {
			return rootHash
		}
	}
	transition, err := ex.BeginTxn(
		rootHash,
		&types.Header{},
		types.ZeroAddress,
	)
	require.NoError(t, err)
	return transition
}
