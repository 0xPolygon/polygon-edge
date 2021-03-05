package jsonrpc

import (
	"testing"

	"github.com/hashicorp/go-hclog"

	"github.com/0xPolygon/minimal/types"
)

// TEST SETUP //

// The idea is to overwrite the methods used by the actual endpoint,
// so we can finely control what gets returned

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

func newMockBlockStore() *mockBlockStore {
	return &mockBlockStore{
		header: &types.Header{Number: 0},
	}
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
		{"Block number out of scope", "5", false},
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
			block, error := dispatcher.endpoints.Eth.GetBlockByNumber(testCase.blockNumber, false)

			if error != nil && !testCase.shouldFail {
				// If there is an error, and the test shouldn't fail
				t.Fatalf("Error: %v", error)
			} else if !testCase.shouldFail {
				foundType, ok := block.(*types.Header)

				if !ok {
					t.Fatalf("Invalid return value for %v. Expected *types.Header, got %v", testCase.name, foundType)
				}
			}
		})
	}
}

func TestBlockNumber(t *testing.T) {
	testTable := []struct {
		name         string
		blockNumbers []uint64
		shouldFail   bool
	}{
		{"Gets the final block number", []uint64{0, 1, 2, 3}, false},
		{"No blocks added", []uint64{}, true},
	}

	store := newMockStore()

	dispatcher := newDispatcher(hclog.NewNullLogger(), store)
	dispatcher.registerEndpoints()

	for _, testCase := range testTable {
		t.Run(testCase.name, func(t *testing.T) {
			block, error := dispatcher.endpoints.Eth.BlockNumber()

			if error != nil && !testCase.shouldFail {
				t.Errorf(testCase.name)
			}

			foundType, ok := block.(uint64)
			if !ok && !testCase.shouldFail {
				t.Errorf("Invalid return value for %v. Expected *types.Header, got %v", testCase.name, foundType)
			}

			if len(testCase.blockNumbers) > 1 && block != testCase.blockNumbers[len(testCase.blockNumbers)-1] {
				t.Errorf("Invalid final block number. Expected %v, got %v", testCase.blockNumbers[len(testCase.blockNumbers)-1], block)
			}
		})
	}
}

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

// TODO GetBalance
// 1. Create a couple of dummy accounts with balances
// 2. Check if the balances match
// Test cases:
// I. Regular matching
// II. Invalid address

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
