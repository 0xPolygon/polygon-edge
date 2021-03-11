package jsonrpc

import (
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"

	"github.com/0xPolygon/minimal/crypto"
	"github.com/0xPolygon/minimal/helper/hex"
	"github.com/0xPolygon/minimal/state"
	"github.com/0xPolygon/minimal/types"
)

// TEST SETUP //

// The idea is to overwrite the methods used by the actual endpoint,
// so we can finely control what gets returned to the test
// Callback functions are functions that should be defined (overwritten) in the test itself

type mockBlockStore struct {
	nullBlockchainInterface

	header *types.Header

	getHeaderByNumberCallback func(blockNumber uint64) (*types.Header, bool)
	addTxCallback             func(tx *types.Transaction) error
	getAvgGasPriceCallback    func() *big.Int

	getAccountCallback func(root types.Hash, addr types.Address) (*state.Account, error)
	getStorageCallback func(root types.Hash, addr types.Address, slot types.Hash) ([]byte, error)
	getCodeCallback    func(hash types.Hash) ([]byte, error)
}

func (m *mockBlockStore) GetAccount(root types.Hash, addr types.Address) (*state.Account, error) {
	return m.getAccountCallback(root, addr)
}

func (m *mockBlockStore) GetStorage(root types.Hash, addr types.Address, slot types.Hash) ([]byte, error) {
	return m.getStorageCallback(root, addr, slot)
}

func (m *mockBlockStore) GetCode(hash types.Hash) ([]byte, error) {
	return m.getCodeCallback(hash)
}

func (m *mockBlockStore) Header() *types.Header {
	return m.header
}

func (m *mockBlockStore) GetHeaderByNumber(blockNumber uint64) (*types.Header, bool) {
	return m.getHeaderByNumberCallback(blockNumber)
}

func (m *mockBlockStore) GetAvgGasPrice() *big.Int {
	return m.getAvgGasPriceCallback()
}

func (m *mockBlockStore) State() state.State {
	return &mockState{}
}

func (m *mockBlockStore) AddTx(tx *types.Transaction) error {
	return m.addTxCallback(tx)
}

type mockStateHelper struct {
}

func (sh *mockStateHelper) NewTxn(state state.State, snapshot state.Snapshot) *state.Txn {
	return nil
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

//func NewTxn(state state.State, snapshot state.Snapshot) *mockTxn {
//	return &mockTxn{}
//}

var signer = &crypto.FrontierSigner{}

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
				assert.IsTypef(t, &types.Header{}, block, "Invalid return type")
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
				assert.Equalf(t, types.Uint64(0), block, "Output not equal")
			}
		})
	}
}

func TestSendRawTransaction(t *testing.T) {
	keys := []*ecdsa.PrivateKey{}
	addresses := []types.Address{}

	// Generate dummy keys and addresses
	for i := 0; i < 3; i++ {
		var key, _ = crypto.GenerateKey()
		var address = crypto.PubKeyToAddress(&key.PublicKey)

		keys = append(keys, key)
		addresses = append(addresses, address)
	}

	testTable := []struct {
		name        string
		transaction *types.Transaction
		key         *ecdsa.PrivateKey
		shouldFail  bool
	}{
		{"Valid raw transaction", &types.Transaction{
			Nonce:    1,
			To:       &addresses[0],
			Value:    []byte{0x1},
			Gas:      10,
			GasPrice: []byte{0x1},
			Input:    []byte{},
		}, keys[0], false},
		{"Invalid from param", &types.Transaction{
			Nonce:    2,
			To:       nil,
			Value:    []byte{0x1},
			Gas:      10,
			GasPrice: []byte{0x1},
			Input:    []byte{},
		}, keys[1], true},
		{"Error when adding to the tx pool", &types.Transaction{
			Nonce:    3,
			To:       &addresses[2],
			Value:    []byte{0x1},
			Gas:      10,
			GasPrice: []byte{0x1},
			Input:    []byte{},
		}, keys[2], true},
	}

	store := newMockBlockStore()

	store.addTxCallback = func(tx *types.Transaction) error {
		return nil
	}

	dispatcher := newTestDispatcher(hclog.NewNullLogger(), store)

	for index, testCase := range testTable {
		if index == len(testTable)-1 {
			store.addTxCallback = func(tx *types.Transaction) error {
				return fmt.Errorf("sample error")
			}
		}

		tx, err := signer.SignTx(testCase.transaction, testCase.key)
		if err != nil {
			t.Fatalf("Error: Unable to sign transaction %v", testCase.transaction)
		}

		t.Run(testCase.name, func(t *testing.T) {
			txHash, txHashError := dispatcher.endpoints.Eth.SendRawTransaction(hex.EncodeToHex(tx.MarshalRLP()))

			if txHashError != nil && !testCase.shouldFail {
				// If there is an error, and the test shouldn't fail
				t.Fatalf("Error: %v", txHashError)
			} else if !testCase.shouldFail {
				assert.IsTypef(t, "", txHash, "Return type mismatch")
			}
		})
	}
}

func TestSendTransaction(t *testing.T) {
	keys := []*ecdsa.PrivateKey{}
	addresses := []types.Address{}

	// Generate dummy keys and addresses
	for i := 0; i < 3; i++ {
		var key, _ = crypto.GenerateKey()
		var address = crypto.PubKeyToAddress(&key.PublicKey)

		keys = append(keys, key)
		addresses = append(addresses, address)
	}

	testTable := []struct {
		name           string
		transactionMap map[string]interface{}
		shouldFail     bool
	}{
		{"Valid transaction object", map[string]interface{}{
			"nonce":    "1",
			"from":     (&addresses[0]).String(),
			"to":       (&addresses[1]).String(),
			"data":     "",
			"gasPrice": "0x0001",
			"gas":      "0x01",
		}, false},
		{"Invalid nonce", map[string]interface{}{
			"nonce":    "",
			"from":     (&addresses[0]).String(),
			"to":       (&addresses[1]).String(),
			"data":     "",
			"gasPrice": "0x0001",
			"gas":      "0x01",
		}, true},
		{"Invalid gas", map[string]interface{}{
			"nonce":    "3",
			"from":     (&addresses[0]).String(),
			"to":       (&addresses[1]).String(),
			"data":     "",
			"gasPrice": "0x0001",
			"gas":      "",
		}, true},
	}

	store := newMockBlockStore()

	store.addTxCallback = func(tx *types.Transaction) error {
		return nil
	}

	dispatcher := newTestDispatcher(hclog.NewNullLogger(), store)

	for _, testCase := range testTable {
		t.Run(testCase.name, func(t *testing.T) {

			if testCase.shouldFail {
				assert.Panicsf(t, func() {
					_, _ = dispatcher.endpoints.Eth.SendTransaction(testCase.transactionMap)
				}, "No panic detected")
			} else {
				assert.NotPanicsf(t, func() {
					txHash, _ := dispatcher.endpoints.Eth.SendTransaction(testCase.transactionMap)

					assert.IsTypef(t, "", txHash, "Return type mismatch")
				}, "Invalid panic detected")
			}
		})
	}
}

func TestGasPrice(t *testing.T) {
	testTable := []struct {
		name       string
		gasPrice   *big.Int
		shouldFail bool
	}{
		{
			"Valid gas price",
			big.NewInt(5),
			false,
		},
	}

	store := newMockBlockStore()

	dispatcher := newTestDispatcher(hclog.NewNullLogger(), store)

	for index, testCase := range testTable {

		store.getAvgGasPriceCallback = func() *big.Int {
			return testTable[index].gasPrice
		}

		t.Run(testCase.name, func(t *testing.T) {
			gasPrice, gasPriceError := dispatcher.endpoints.Eth.GasPrice()

			if testCase.shouldFail {
				assert.NotNilf(t, gasPriceError, "Invalid fail case")
			} else {
				assert.IsTypef(t, "", gasPrice, "Invalid return type")

				assert.Equalf(t, gasPrice, hex.EncodeBig(testCase.gasPrice), "Return value doesn't match")
			}
		})
	}
}

// TODO add before each to the test suite

//func TestGetBalance(t *testing.T) {
//	balances := []*big.Int{big.NewInt(10), big.NewInt(15)}
//
//	testTable := []struct {
//		name       string
//		address    string
//		balance    *big.Int
//		shouldFail bool
//	}{
//		{"Balances match for account 1", "1", balances[0], false},
//		{"Balances match for account 2", "2", balances[1], true},
//		{"Invalid account address", "3", nil, true},
//	}
//
//	// Setup //
//	store := newMockBlockStore()
//	storeState := store.State()
//	snap, _ := storeState.NewSnapshotAt(store.header.StateRoot)
//
//	stateHelper := store.GetStateHelper()
//
//	txn := stateHelper.NewTxn(storeState, snap)
//	txn.accounts = map[types.Address]*mockAccount{}
//	txn.accounts[types.StringToAddress("1")] = &mockAccount{
//		Acct: &state.Account{
//			Nonce:   uint64(123),
//			Balance: balances[0],
//		},
//		Storage: nil,
//	}
//	txn.accounts[types.StringToAddress("2")] = &mockAccount{
//		Acct: &state.Account{
//			Nonce:   uint64(456),
//			Balance: balances[1],
//		},
//		Storage: nil,
//	}
//
//	dispatcher := newTestDispatcher(hclog.NewNullLogger(), store)
//
//	for _, testCase := range testTable {
//
//		t.Run(testCase.name, func(t *testing.T) {
//			balance, balanceError := dispatcher.endpoints.Eth.GetBalance(testCase.address, LatestBlockNumber)
//
//			if balanceError != nil && !testCase.shouldFail {
//				// If there is an error, and the test shouldn't fail
//				t.Fatalf("Error: %v", balanceError)
//			} else if !testCase.shouldFail {
//				AssertEqual(t, balance, testCase.balance, true)
//			}
//		})
//	}
//}

// TODO
// GetBlockByHash
// GetTransactionReceipt

// Remaining test methods:

// TODO Call
// 1. Deploy a dummy contract with 1 method
// 2. Call the contract method and check result
// Test cases:
// I. Regular call
// II. Call for non existing block num

// TODO GetLogs
// 1. Create dummy blocks with receipts / logs
// 2. Create a dummy filter that matches a subset
// 3. Check if it returns the matched logs
// Test cases:
// I. Regular case
// II. No logs found

// TODO GetStorageAt (uses state)
// 1. Create a dummy contract with 1 field
// 2. Check if the storage matches
// Test cases:
// I. Normal creation
// II. Invalid header
// III. Invalid index

// TODO EstimateGas (uses state)

// TODO GetTransactionCount (uses state)
// 1. Create two accounts
// 2. Send a couple of transactions from 1 account to the other
// 3. Check the nonce
// Test cases:
// I. Regular case
// II. Invalid block
// III. Invalid address

// TODO GetCode (uses state)
// 1. Create a dummy contract
// 2. Fetch the code
// Test cases:
// I. User account
// II. Contract account
// III. Invalid address
// IV. Invalid block number
