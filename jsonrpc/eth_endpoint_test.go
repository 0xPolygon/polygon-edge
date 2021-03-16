package jsonrpc

import (
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"strconv"
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

	header       *types.Header
	mockAccounts map[types.Address]*mockAccount

	getHeaderByNumberCallback func(blockNumber uint64) (*types.Header, bool)
	addTxCallback             func(tx *types.Transaction) error
	getAvgGasPriceCallback    func() *big.Int

	getAccountCallback        func(root types.Hash, addr types.Address) (*state.Account, error)
	getStorageCallback        func(root types.Hash, addr types.Address, slot types.Hash) ([]byte, error)
	getCodeCallback           func(hash types.Hash) ([]byte, error)
	applyTxnCallback          func(header *types.Header, txn *types.Transaction) ([]byte, bool, error)
	getReceiptsByHashCallback func(hash types.Hash) ([]*types.Receipt, error)
}

func (m *mockBlockStore) GetAccount(root types.Hash, addr types.Address) (*state.Account, error) {
	return m.getAccountCallback(root, addr)
}

func (m *mockBlockStore) GetReceiptsByHash(hash types.Hash) ([]*types.Receipt, error) {
	return m.getReceiptsByHashCallback(hash)
}

func (m *mockBlockStore) GetStorage(root types.Hash, addr types.Address, slot types.Hash) ([]byte, error) {
	return m.getStorageCallback(root, addr, slot)
}

func (m *mockBlockStore) ApplyTxn(header *types.Header, txn *types.Transaction) ([]byte, bool, error) {
	return m.applyTxnCallback(header, txn)
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

func newMockBlockStore() *mockBlockStore {
	return &mockBlockStore{
		header: &types.Header{Number: 0},
	}
}

// DUMMY DATA GENERATION //

func (m *mockBlockStore) generateTopics(numTopics int) []types.Hash {
	var topics = []types.Hash{}

	for i := 0; i < numTopics; i++ {
		topics = append(topics, types.StringToHash(fmt.Sprintf("topic %d", i)))
	}

	return topics
}

func (m *mockBlockStore) generateLogs(numLogs int, numTopics int) []*types.Log {

	var logs = []*types.Log{}

	for i := 0; i < numLogs; i++ {
		logs = append(logs, &types.Log{
			Topics: m.generateTopics(numTopics),
		})
	}

	return logs
}

type generateReceiptsParams struct {
	numReceipts int
	numLogs     int
	numTopics   int
}

func (m *mockBlockStore) generateReceipts(params generateReceiptsParams) []*types.Receipt {

	var receipts = []*types.Receipt{}

	for i := 0; i < params.numReceipts; i++ {
		receipts = append(receipts, &types.Receipt{
			Logs: m.generateLogs(params.numLogs, params.numTopics),
		})
	}

	return receipts
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

	store.mockAccounts = map[types.Address]*mockAccount{}
	store.mockAccounts[types.StringToAddress("1")] = &mockAccount{
		Acct: &state.Account{
			Nonce:   uint64(123),
			Balance: balances[0],
		},
		Storage: nil,
	}

	store.mockAccounts[types.StringToAddress("2")] = &mockAccount{
		Acct: &state.Account{
			Nonce:   uint64(456),
			Balance: balances[1],
		},
		Storage: nil,
	}

	store.getAccountCallback = func(root types.Hash, addr types.Address) (*state.Account, error) {
		if val, ok := store.mockAccounts[addr]; ok {
			return val.Acct, nil
		}

		return nil, fmt.Errorf("no account found")
	}

	dispatcher := newTestDispatcher(hclog.NewNullLogger(), store)

	for _, testCase := range testTable {

		t.Run(testCase.name, func(t *testing.T) {
			balance, balanceError := dispatcher.endpoints.Eth.GetBalance(testCase.address, LatestBlockNumber)

			if balanceError != nil && !testCase.shouldFail {
				// If there is an error, and the test shouldn't fail
				t.Fatalf("Error: %v", balanceError)
			} else if !testCase.shouldFail {
				assert.Equalf(t, (*types.Big)(testCase.balance), balance, "Balances don't match")
			}
		})
	}
}

func TestGetTransactionCount(t *testing.T) {
	testTable := []struct {
		name        string
		address     string
		nonce       uint64
		blockNumber BlockNumber
		shouldPanic bool
		shouldFail  bool
	}{
		{"Valid address nonce", "1", uint64(123), LatestBlockNumber, false, false},
		{"Invalid address", "5", 0, LatestBlockNumber, false, true},
		{"Invalid block number", "2", 0, -50, true, true},
	}

	// Setup //
	store := newMockBlockStore()

	store.mockAccounts = map[types.Address]*mockAccount{}
	store.mockAccounts[types.StringToAddress("1")] = &mockAccount{
		Acct: &state.Account{
			Nonce:   uint64(123),
			Balance: nil,
		},
		Storage: nil,
	}

	store.mockAccounts[types.StringToAddress("2")] = &mockAccount{
		Acct: &state.Account{
			Nonce:   uint64(456),
			Balance: nil,
		},
		Storage: nil,
	}

	store.getAccountCallback = func(root types.Hash, addr types.Address) (*state.Account, error) {
		if val, ok := store.mockAccounts[addr]; ok {
			return val.Acct, nil
		}

		return nil, fmt.Errorf("no account found")
	}

	dispatcher := newTestDispatcher(hclog.NewNullLogger(), store)

	for _, testCase := range testTable {

		t.Run(testCase.name, func(t *testing.T) {
			if testCase.shouldPanic {
				assert.Panicsf(t, func() {
					_, _ = dispatcher.endpoints.Eth.GetTransactionCount(testCase.address, testCase.blockNumber)
				}, "No panic detected")
			} else {
				nonce, nonceError := dispatcher.endpoints.Eth.GetTransactionCount(testCase.address, testCase.blockNumber)

				if nonceError != nil && !testCase.shouldFail {
					// If there is an error, and the test shouldn't fail
					t.Fatalf("Error: %v", nonceError)
				} else if !testCase.shouldFail {
					assert.Equalf(t, (types.Uint64)(testCase.nonce), nonce, "Nonces don't match")
				}
			}
		})
	}
}

func TestGetCode(t *testing.T) {
	testTable := []struct {
		name        string
		address     string
		shouldPanic bool
		shouldFail  bool
	}{
		{"Valid address code", "1", false, false},
		{"Invalid code", "2", false, true},
	}

	// Setup //
	store := newMockBlockStore()

	store.mockAccounts = map[types.Address]*mockAccount{}
	store.mockAccounts[types.StringToAddress("1")] = &mockAccount{
		Acct: &state.Account{
			Nonce:    uint64(123),
			Balance:  nil,
			CodeHash: types.StringToAddress("123").Bytes(),
		},
		Storage: nil,
	}

	store.mockAccounts[types.StringToAddress("2")] = &mockAccount{
		Acct: &state.Account{
			Nonce:    uint64(456),
			Balance:  nil,
			CodeHash: nil,
		},
		Storage: nil,
	}

	store.getAccountCallback = func(root types.Hash, addr types.Address) (*state.Account, error) {
		if val, ok := store.mockAccounts[addr]; ok {
			return val.Acct, nil
		}

		return nil, fmt.Errorf("no account found")
	}

	dispatcher := newTestDispatcher(hclog.NewNullLogger(), store)

	for index, testCase := range testTable {

		if index == 1 {
			store.getCodeCallback = func(hash types.Hash) ([]byte, error) {
				return nil, fmt.Errorf("invalid hash")
			}
		} else {
			store.getCodeCallback = func(hash types.Hash) ([]byte, error) {
				return []byte{}, nil
			}
		}

		t.Run(testCase.name, func(t *testing.T) {
			if testCase.shouldPanic {
				assert.Panicsf(t, func() {
					_, _ = dispatcher.endpoints.Eth.GetCode(testCase.address, LatestBlockNumber)
				}, "No panic detected")
			} else {
				code, codeError := dispatcher.endpoints.Eth.GetCode(testCase.address, LatestBlockNumber)

				if codeError != nil && !testCase.shouldFail {
					// If there is an error, and the test shouldn't fail
					t.Fatalf("Error: %v", codeError)
				} else if !testCase.shouldFail {
					assert.IsTypef(t,
						types.HexBytes{},
						code,
						"Code hashes don't match")
				}
			}
		})
	}
}

func TestGetStorageAt(t *testing.T) {
	testTable := []struct {
		name        string
		address     string
		index       types.Hash
		shouldPanic bool
		shouldFail  bool
	}{
		{"Valid address", "1", types.StringToHash("1"), false, false},
		{"Invalid address", "2", types.StringToHash("1"), false, true},
		{"Invalid index", "1", types.StringToHash("2"), false, true},
	}

	// Setup //
	store := newMockBlockStore()

	store.mockAccounts = map[types.Address]*mockAccount{}
	store.mockAccounts[types.StringToAddress("1")] = &mockAccount{
		Acct: &state.Account{
			Nonce:    uint64(123),
			Balance:  nil,
			CodeHash: types.StringToAddress("123").Bytes(),
		},
		Storage: map[types.Hash]types.Hash{
			types.StringToHash("1"): types.StringToHash("Slot 1 ACC 1"),
		},
	}

	store.getStorageCallback = func(root types.Hash, addr types.Address, slot types.Hash) ([]byte, error) {

		if addr != types.StringToAddress("1") {
			// Simulate a failed account fetch
			return nil, fmt.Errorf("invalid address")
		}

		if slot != types.StringToHash("1") {
			// Simulate invalid slot access
			return nil, fmt.Errorf("")
		}

		return store.mockAccounts[addr].Storage[slot].Bytes(), nil
	}

	dispatcher := newTestDispatcher(hclog.NewNullLogger(), store)

	for _, testCase := range testTable {
		t.Run(testCase.name, func(t *testing.T) {
			if testCase.shouldPanic {
				assert.Panicsf(t, func() {
					_, _ = dispatcher.endpoints.Eth.GetStorageAt(
						testCase.address,
						testCase.index,
						LatestBlockNumber,
					)
				}, "No panic detected")
			} else {
				slot, slotError := dispatcher.endpoints.Eth.GetStorageAt(
					testCase.address,
					testCase.index,
					LatestBlockNumber,
				)

				if slotError != nil && !testCase.shouldFail {
					// If there is an error, and the test shouldn't fail
					t.Fatalf("Error: %v", slotError)
				} else if !testCase.shouldFail {
					assert.Equalf(
						t,
						types.StringToHash("Slot 1 ACC 1").Bytes(),
						slot,
						"Slot doesn't match",
					)
				}
			}
		})
	}
}

func TestCall(t *testing.T) {
	var key, _ = crypto.GenerateKey()
	var address = crypto.PubKeyToAddress(&key.PublicKey)

	dummyTransaction := &types.Transaction{
		Nonce:    1,
		To:       &address,
		Value:    []byte{0x1},
		Gas:      10,
		GasPrice: []byte{0x1},
		Input:    []byte{},
	}

	testTable := []struct {
		name        string
		transaction *types.Transaction
		shouldPanic bool
		shouldFail  bool
	}{
		{"Valid return value", dummyTransaction, false, false},
		{"Failed transaction", dummyTransaction, false, true},
		{"Invalid transaction", dummyTransaction, false, true},
	}

	// Setup //
	store := newMockBlockStore()

	// Used for return value comparison
	referenceReturnValue := types.Hex2Bytes("0x0005")

	dispatcher := newTestDispatcher(hclog.NewNullLogger(), store)

	for index, testCase := range testTable {

		switch index {
		case 0:
			store.applyTxnCallback = func(header *types.Header, txn *types.Transaction) ([]byte, bool, error) {
				return referenceReturnValue, false, nil
			}
		case 1:
			store.applyTxnCallback = func(header *types.Header, txn *types.Transaction) ([]byte, bool, error) {
				return nil, true, nil
			}
		case 2:
			store.applyTxnCallback = func(header *types.Header, txn *types.Transaction) ([]byte, bool, error) {
				return nil, false, fmt.Errorf("Unable to apply transaction")
			}
		}

		t.Run(testCase.name, func(t *testing.T) {
			if testCase.shouldPanic {
				assert.Panicsf(t, func() {
					_, _ = dispatcher.endpoints.Eth.Call(
						testCase.transaction,
						LatestBlockNumber,
					)
				}, "No panic detected")
			} else {
				returnValue, callError := dispatcher.endpoints.Eth.Call(
					testCase.transaction,
					LatestBlockNumber,
				)

				if callError != nil && !testCase.shouldFail {
					// If there is an error, and the test shouldn't fail
					t.Fatalf("Error: %v", callError)
				} else if !testCase.shouldFail {
					assert.Equalf(
						t,
						referenceReturnValue,
						returnValue,
						"Return values don't match",
					)
				}
			}
		})
	}
}

func TestGetLogs(t *testing.T) {

	// Topics we're searching for
	var topics = [][]types.Hash{
		{types.StringToHash("topic 4")},
		{types.StringToHash("topic 5")},
		{types.StringToHash("topic 6")},
	}

	testTable := []struct {
		name          string
		filterOptions *LogFilter
		shouldFail    bool
	}{
		{"Found matching logs",
			&LogFilter{
				fromBlock: 1,
				toBlock:   4,
				Topics:    topics,
			},
			false},
		{"Invalid block range", &LogFilter{
			fromBlock: 10,
			toBlock:   5,
			Topics:    topics,
		}, true},
		{"Reference blocks not found", &LogFilter{
			fromBlock: 10,
			toBlock:   20,
			Topics:    topics,
		}, true},
	}

	// Setup //
	store := newMockBlockStore()

	dispatcher := newTestDispatcher(hclog.NewNullLogger(), store)

	store.getHeaderByNumberCallback = func(blockNumber uint64) (*types.Header, bool) {
		return &types.Header{
			Number:       blockNumber,
			ReceiptsRoot: types.StringToHash(strconv.FormatUint(blockNumber, 10)),
		}, true
	}

	store.getReceiptsByHashCallback = func(hash types.Hash) ([]*types.Receipt, error) {

		switch hash {
		case types.StringToHash(strconv.FormatUint(1, 10)):
			return store.generateReceipts(
				generateReceiptsParams{
					numReceipts: 1,
					numTopics:   1,
					numLogs:     2,
				},
			), nil
		case types.StringToHash(strconv.FormatUint(2, 10)):
			return store.generateReceipts(
				generateReceiptsParams{
					numReceipts: 2,
					numTopics:   5,
					numLogs:     1,
				},
			), nil
		case types.StringToHash(strconv.FormatUint(3, 10)):
			return store.generateReceipts(
				generateReceiptsParams{
					numReceipts: 3,
					numTopics:   10,
					numLogs:     5,
				},
			), nil
		}

		return nil, nil
	}

	for index, testCase := range testTable {

		if index == 2 {
			store.getHeaderByNumberCallback = func(blockNumber uint64) (*types.Header, bool) {
				return nil, false
			}
		}

		t.Run(testCase.name, func(t *testing.T) {
			foundLogs, logError := dispatcher.endpoints.Eth.GetLogs(
				testCase.filterOptions,
			)

			if logError != nil && !testCase.shouldFail {
				// If there is an error, and the test shouldn't fail
				t.Fatalf("Error: %v", logError)
			} else if !testCase.shouldFail {
				assert.Lenf(t, foundLogs, 17, "Invalid number of logs found")
			}
		})
	}
}
