package jsonrpc

import (
	"math/big"
	"reflect"
	"testing"

	"github.com/hashicorp/go-hclog"

	"github.com/0xPolygon/minimal/state"
	"github.com/0xPolygon/minimal/types"
)

// HELPER FUNCTIONS //

// AssertEqual checks if values are equal
func AssertEqual(t *testing.T, a interface{}, b interface{}, fatal bool) {
	if a == b {
		return
	}

	if fatal {
		t.Fatalf("Received %v (type %v), expected %v (type %v)", a, reflect.TypeOf(a), b, reflect.TypeOf(b))
	} else {
		t.Errorf("Received %v (type %v), expected %v (type %v)", a, reflect.TypeOf(a), b, reflect.TypeOf(b))
	}

}

// AssertType checks if a is the type of b
func AssertType(t *testing.T, a interface{}, b reflect.Type, fatal bool) {
	if reflect.TypeOf(a) != b {
		if fatal {
			t.Fatalf("Received %v (type %v), expected type %v", a, reflect.TypeOf(a), b)
		} else {
			t.Errorf("Received %v (type %v), expected type %v", a, reflect.TypeOf(a), b)
		}
	}
}

// TEST SETUP //

// The idea is to overwrite the methods used by the actual endpoint,
// so we can finely control what gets returned to the test
// Callback functions are functions that should be defined (overwritten) in the test itself

type mockBlockStore struct {
	nullBlockchainInterface

	header *types.Header

	getHeaderByNumberCallback func(blockNumber uint64) (*types.Header, bool)
}

func (m *mockBlockStore) Header() *types.Header {
	return m.header
}

func (m *mockBlockStore) GetHeaderByNumber(blockNumber uint64) (*types.Header, bool) {
	return m.getHeaderByNumberCallback(blockNumber)
}

func (m *mockBlockStore) State() state.State {
	return &mockState{}
}

func newMockBlockStore() *mockBlockStore {
	return &mockBlockStore{
		header: &types.Header{Number: 0},
	}
}

// STATE / SNAPSHOT / ACCOUNTS MOCKS //

type nullStateInterface struct {
}

func (b *nullStateInterface) NewSnapshotAt(types.Hash) (state.Snapshot, error) {
	return nil, nil
}

func (b *nullStateInterface) NewSnapshot() state.Snapshot {
	return nil
}

func (b *nullStateInterface) GetCode(hash types.Hash) ([]byte, bool) {
	return nil, false
}

type mockAccount struct {
	Acct    *state.Account
	Storage map[types.Hash]types.Hash
}

type mockState struct {
	nullStateInterface

	newSnapshotAtCallback func(types.Hash) (state.Snapshot, error)
	newSnapshotCallback   func() state.Snapshot
	getCodeCallback       func(hash types.Hash) ([]byte, bool)
}

type mockTxn struct {
	accounts map[types.Address]*mockAccount
}

func (m *mockTxn) GetAccount(addr types.Address) (*state.Account, bool) {
	if val, ok := m.accounts[addr]; ok {
		return val.Acct, true
	}

	return nil, false
}

type mockSnapshot struct {
}

func (m *mockSnapshot) Get(k []byte) ([]byte, bool) {
	return nil, false
}

func (m *mockSnapshot) Commit(objs []*state.Object) (state.Snapshot, []byte) {
	return nil, nil
}

func (m *mockState) NewSnapshotAt(hash types.Hash) (state.Snapshot, error) {
	return &mockSnapshot{}, nil
}

func (m *mockState) NewSnapshot() state.Snapshot {
	return &mockSnapshot{}
}

func (m *mockState) GetCode(hash types.Hash) ([]byte, bool) {
	return m.getCodeCallback(hash)
}

func NewTxn(state state.State, snapshot state.Snapshot) *mockTxn {
	return &mockTxn{}
}

// TESTS //

func TestGetBlockByNumber(t *testing.T) {
	testTable := []struct {
		name        string
		blockNumber string
		shouldFail  bool
	}{
		{"Get latest block", "latest", false},
		{"Get the earliest block", "earliest", true},
		{"Invalid block number", "-50", true},
		{"Empty block number", "", true},
		{"Valid block number", "2", false},
		{"Block number out of scope", "6", true},
	}

	store := newMockBlockStore()

	store.getHeaderByNumberCallback = func(blockNumber uint64) (*types.Header, bool) {
		if blockNumber > 5 {
			return nil, false
		} else {
			return &types.Header{Number: blockNumber}, true
		}
	}

	dispatcher := newTestDispatcher(hclog.NewNullLogger(), store)

	for _, testCase := range testTable {
		t.Run(testCase.name, func(t *testing.T) {
			block, blockError := dispatcher.endpoints.Eth.GetBlockByNumber(testCase.blockNumber, false)

			if blockError != nil && !testCase.shouldFail {
				// If there is an error, and the test shouldn't fail
				t.Fatalf("Error: %v", blockError)
			} else if !testCase.shouldFail {
				AssertType(t, block, reflect.TypeOf(&types.Header{}), true)
			}
		})
	}
}

func TestBlockNumber(t *testing.T) {
	testTable := []struct {
		name        string
		blockNumber uint64
		shouldFail  bool
	}{
		{"Gets the final block number", 0, false},
		{"No blocks added", 0, true},
	}

	store := newMockBlockStore()

	dispatcher := newTestDispatcher(hclog.NewNullLogger(), store)

	for index, testCase := range testTable {
		if index == 1 {
			store.header = nil
		}

		t.Run(testCase.name, func(t *testing.T) {
			block, blockError := dispatcher.endpoints.Eth.BlockNumber()

			if blockError != nil && !testCase.shouldFail {
				// If there is an error, and the test shouldn't fail
				t.Fatalf("Error: %v", blockError)
			} else if !testCase.shouldFail {
				AssertEqual(t, block, types.Uint64(0), true)
			}
		})
	}
}

func TestGetBalance(t *testing.T) {
	balances := []*big.Int{big.NewInt(10), big.NewInt(15)}

	testTable := []struct {
		name       string
		address    string
		balance    *big.Int
		shouldFail bool
	}{
		{"Balances match for account 1", "1", balances[0], false},
		{"Balances match for account 2", "2", balances[1], true},
		{"Invalid account address", "3", nil, true},
	}

	// Setup //
	store := newMockBlockStore()
	storeState := store.State()
	snap, _ := storeState.NewSnapshotAt(store.header.StateRoot)

	txn := NewTxn(storeState, snap)
	txn.accounts = map[types.Address]*mockAccount{}
	txn.accounts[types.StringToAddress("1")] = &mockAccount{
		Acct: &state.Account{
			Nonce:   uint64(123),
			Balance: balances[0],
		},
		Storage: nil,
	}
	txn.accounts[types.StringToAddress("2")] = &mockAccount{
		Acct: &state.Account{
			Nonce:   uint64(456),
			Balance: balances[1],
		},
		Storage: nil,
	}

	dispatcher := newTestDispatcher(hclog.NewNullLogger(), store)

	for _, testCase := range testTable {

		t.Run(testCase.name, func(t *testing.T) {
			balance, balanceError := dispatcher.endpoints.Eth.GetBalance(testCase.address, LatestBlockNumber)

			if balanceError != nil && !testCase.shouldFail {
				// If there is an error, and the test shouldn't fail
				t.Fatalf("Error: %v", balanceError)
			} else if !testCase.shouldFail {
				AssertEqual(t, balance, testCase.balance, true)
			}
		})
	}
}

// TODO
// GetBlockByHash
// GetTransactionReceipt

// Remaining test methods:
// TODO SendRawTransaction
// 1. Create a transaction
// 2. Sign it with a dummy key
// 3. Check if it returns a hash
// Test cases:
// I. Normal raw transaction
// II. Empty input
// III. Missing from / Signature

// TODO SendTransaction
// 1. Create transaction with params
// 2. Check if it returns a hash
// Test cases:
// I. Normal transaction
// II. Missing fields

// TODO GetStorageAt
// 1. Create a dummy contract with 1 field
// 2. Check if the storage matches
// Test cases:
// I. Normal creation
// II. Invalid header
// III. Invalid index

// TODO GasPrice
// 1. Create a couple of blocks with a gas price
// 2. Get the average and check if it matches
// Test cases:
// I. Regular average (multiple blocks)
// II. Single block
// III. No blocks

// TODO Call
// 1. Deploy a dummy contract with 1 method
// 2. Call the contract method and check result
// Test cases:
// I. Regular call
// II. Call for non existing block num

// TODO GetBlockHeader
// 1. Call with block string / number
// 2. Check if the response matches
// Test cases:
// I. Latest
// II. Earliest
// III. Pending
// IV. Specific block number
// V. Block number out of bounds

// TODO EstimateGas

// TODO GetLogs
// 1. Create dummy blocks with receipts / logs
// 2. Create a dummy filter that matches a subset
// 3. Check if it returns the matched logs
// Test cases:
// I. Regular case
// II. No logs found

// TODO GetTransactionCount
// 1. Create two accounts
// 2. Send a couple of transactions from 1 account to the other
// 3. Check the nonce
// Test cases:
// I. Regular case
// II. Invalid block
// III. Invalid address

// TODO GetCode
// 1. Create a dummy contract
// 2. Fetch the code
// Test cases:
// I. User account
// II. Contract account
// III. Invalid address
// IV. Invalid block number
