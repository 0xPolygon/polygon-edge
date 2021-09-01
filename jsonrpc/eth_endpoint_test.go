package jsonrpc

import (
	"bytes"
	"fmt"
	"math/big"
	"strconv"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"

	"github.com/0xPolygon/polygon-sdk/helper/hex"
	"github.com/0xPolygon/polygon-sdk/state"
	"github.com/0xPolygon/polygon-sdk/types"
)

type mockAccount2 struct {
	store   *mockAccountStore
	address types.Address
	code    []byte
	account *state.Account
	storage map[types.Hash]types.Hash
}

func (m *mockAccount2) Storage(k, v types.Hash) {
	m.storage[k] = v
}

func (m *mockAccount2) Code(code []byte) {
	codeHash := types.BytesToHash(m.address.Bytes())
	m.code = code
	m.account.CodeHash = codeHash.Bytes()
}

func (m *mockAccount2) Nonce(n uint64) {
	m.account.Nonce = n
}

func (m *mockAccount2) Balance(n uint64) {
	m.account.Balance = new(big.Int).SetUint64(n)
}

type mockAccountStore struct {
	nullBlockchainInterface
	accounts map[types.Address]*mockAccount2
}

func (m *mockAccountStore) AddAccount(addr types.Address) *mockAccount2 {
	if m.accounts == nil {
		m.accounts = map[types.Address]*mockAccount2{}
	}
	acct := &mockAccount2{
		store:   m,
		address: addr,
		account: &state.Account{},
		storage: map[types.Hash]types.Hash{},
	}
	m.accounts[addr] = acct
	return acct
}

func (m *mockAccountStore) Header() *types.Header {
	return &types.Header{}
}

func (m *mockAccountStore) GetAccount(root types.Hash, addr types.Address) (*state.Account, error) {
	acct, ok := m.accounts[addr]
	if !ok {
		return nil, ErrStateNotFound
	}
	return acct.account, nil
}

func (m *mockAccountStore) GetCode(hash types.Hash) ([]byte, error) {
	for _, acct := range m.accounts {
		if bytes.Equal(acct.account.CodeHash, hash.Bytes()) {
			return acct.code, nil
		}
	}
	return nil, fmt.Errorf("code not found")
}

func (m *mockAccountStore) GetStorage(root types.Hash, addr types.Address, slot types.Hash) ([]byte, error) {
	acct, ok := m.accounts[addr]
	if !ok {
		return nil, ErrStateNotFound
	}
	val, ok := acct.storage[slot]
	if !ok {
		return nil, ErrStateNotFound
	}
	return val.Bytes(), nil
}

type mockBlockStore2 struct {
	nullBlockchainInterface
	blocks []*types.Block
	topics []types.Hash
}

func (m *mockBlockStore2) add(blocks ...*types.Block) {
	if m.blocks == nil {
		m.blocks = []*types.Block{}
	}
	m.blocks = append(m.blocks, blocks...)
}

func (m *mockBlockStore2) GetReceiptsByHash(hash types.Hash) ([]*types.Receipt, error) {
	switch hash {
	case types.StringToHash(strconv.Itoa(1)):
		return []*types.Receipt{
			{
				Logs: []*types.Log{
					{
						Topics: []types.Hash{
							hash1,
						},
					},
					{
						Topics: m.topics,
					},
				},
			},
		}, nil
	case types.StringToHash(strconv.Itoa(2)):
		return []*types.Receipt{
			{
				Logs: []*types.Log{
					{
						Topics: []types.Hash{
							hash1, hash2, hash3,
						},
					},
				},
			},
			{
				Logs: []*types.Log{
					{
						Topics: m.topics,
					},
				},
			},
		}, nil
	case types.StringToHash(strconv.Itoa(3)):
		return []*types.Receipt{
			{
				Logs: []*types.Log{
					{
						Topics: m.topics,
					},
				},
			},
			{
				Logs: []*types.Log{
					{
						Topics: []types.Hash{
							hash1, hash2, hash3,
						},
					},
				},
			},
			{
				Logs: []*types.Log{
					{
						Topics: []types.Hash{
							hash1, hash2, hash3,
						},
					},
				},
			},
		}, nil
	}

	return nil, nil
}

func (m *mockBlockStore2) GetHeaderByNumber(blockNumber uint64) (*types.Header, bool) {
	b, ok := m.GetBlockByNumber(blockNumber, false)
	if !ok {
		return nil, false
	}
	return b.Header, true
}

func (m *mockBlockStore2) GetBlockByNumber(blockNumber uint64, full bool) (*types.Block, bool) {
	for _, b := range m.blocks {
		if b.Number() == blockNumber {
			return b, true
		}
	}
	return nil, false
}

func (m *mockBlockStore2) GetBlockByHash(hash types.Hash, full bool) (*types.Block, bool) {
	for _, b := range m.blocks {
		if b.Hash() == hash {
			return b, true
		}
	}
	return nil, false
}

func (m *mockBlockStore2) Header() *types.Header {
	return m.blocks[len(m.blocks)-1].Header
}

func TestEth_Block_GetBlockByNumber(t *testing.T) {
	store := &mockBlockStore2{}
	for i := 0; i < 10; i++ {
		store.add(&types.Block{
			Header: &types.Header{
				Number: uint64(i),
			},
		})
	}

	dispatcher := newTestDispatcher(hclog.NewNullLogger(), store)

	cases := []struct {
		blockNum BlockNumber
		isNotNil bool
		err      bool
	}{
		{LatestBlockNumber, true, false},
		{EarliestBlockNumber, false, true},
		{BlockNumber(-50), false, true},
		{BlockNumber(0), true, false},
		{BlockNumber(2), true, false},
		{BlockNumber(50), false, false},
	}
	for _, c := range cases {
		res, err := dispatcher.endpoints.Eth.GetBlockByNumber(c.blockNum, false)

		if c.isNotNil {
			assert.NotNil(t, res, "expected to return block, but got nil")
		} else {
			assert.Nil(t, res, "expected to return nil, but got data")
		}

		if c.err {
			assert.Error(t, err)
		} else {
			assert.NoError(t, err)
		}
	}
}

func TestEth_Block_GetBlockByHash(t *testing.T) {
	store := &mockBlockStore2{}
	store.add(&types.Block{
		Header: &types.Header{
			Hash: hash1,
		},
	})

	dispatcher := newTestDispatcher(hclog.NewNullLogger(), store)

	res, err := dispatcher.endpoints.Eth.GetBlockByHash(hash1, false)
	assert.NoError(t, err)
	assert.NotNil(t, res)

	res, err = dispatcher.endpoints.Eth.GetBlockByHash(hash2, false)
	assert.NoError(t, err)
	assert.Nil(t, res)
}

func TestEth_Block_BlockNumber(t *testing.T) {
	store := &mockBlockStore2{}
	store.add(&types.Block{
		Header: &types.Header{
			Number: 10,
		},
	})
	dispatcher := newTestDispatcher(hclog.NewNullLogger(), store)

	num, err := dispatcher.endpoints.Eth.BlockNumber()
	assert.NoError(t, err)
	assert.Equal(t, argUintPtr(10), num)
}

func TestEth_Block_GetLogs(t *testing.T) {

	// Topics we're searching for
	topic1 := types.StringToHash("4")
	topic2 := types.StringToHash("5")
	topic3 := types.StringToHash("6")
	var topics = [][]types.Hash{{topic1}, {topic2}, {topic3}}

	testTable := []struct {
		name           string
		filterOptions  *LogFilter
		shouldFail     bool
		expectedLength int
	}{
		{"Found matching logs, fromBlock < toBlock",
			&LogFilter{
				fromBlock: 1,
				toBlock:   3,
				Topics:    topics,
			},
			false, 3},
		{"Found matching logs, fromBlock == toBlock",
			&LogFilter{
				fromBlock: 2,
				toBlock:   2,
				Topics:    topics,
			},
			false, 1},
		{"No logs found", &LogFilter{
			fromBlock: 4,
			toBlock:   5,
			Topics:    topics,
		}, false, 0},
		{"Invalid block range", &LogFilter{
			fromBlock: 10,
			toBlock:   5,
			Topics:    topics,
		}, true, 0},
	}

	// setup test
	store := &mockBlockStore2{}
	store.topics = []types.Hash{topic1, topic2, topic3}
	for i := 0; i < 4; i++ {
		store.add(&types.Block{
			Header: &types.Header{
				Number: uint64(i),
				Hash:   types.StringToHash(strconv.Itoa(i)),
			},
		})
	}
	dispatcher := newTestDispatcher(hclog.NewNullLogger(), store)

	for _, testCase := range testTable {
		t.Run(testCase.name, func(t *testing.T) {
			foundLogs, logError := dispatcher.endpoints.Eth.GetLogs(
				testCase.filterOptions,
			)

			if logError != nil && !testCase.shouldFail {
				// If there is an error and test isn't expected to fail
				t.Fatalf("Error: %v", logError)
			} else if !testCase.shouldFail {
				switch testCase.name {
				case "Found matching logs, fromBlock < toBlock":
					assert.Lenf(t, foundLogs, testCase.expectedLength, "Invalid number of logs found")
				case "Found matching logs, fromBlock == toBlock":
					assert.Lenf(t, foundLogs, testCase.expectedLength, "Invalid number of logs found")
				case "No logs found":
					assert.Lenf(t, foundLogs, testCase.expectedLength, "Invalid number of logs found")
				}
			}
		})
	}
}

var (
	addr0                = types.Address{0x1}
	uninitializedAddress = types.Address{0x99}
)

func TestEth_State_GetBalance(t *testing.T) {
	store := &mockAccountStore{}

	acct0 := store.AddAccount(addr0)
	acct0.Balance(100)

	dispatcher := newTestDispatcher(hclog.NewNullLogger(), store)

	balance, err := dispatcher.endpoints.Eth.GetBalance(addr0, LatestBlockNumber)
	assert.NoError(t, err)
	assert.Equal(t, balance, argBigPtr(big.NewInt(100)))

	// address not found
	balance, err = dispatcher.endpoints.Eth.GetBalance(addr1, LatestBlockNumber)
	assert.NoError(t, err)
	assert.Equal(t, balance, argUintPtr(0))
}

func TestEth_State_GetTransactionCount(t *testing.T) {
	tests := []struct {
		name          string
		initialNonces map[types.Address]uint64
		target        types.Address
		blockNumber   BlockNumber
		succeeded     bool
		expectedNonce *argUint64
	}{
		{
			name: "should return nonce for existing account",
			initialNonces: map[types.Address]uint64{
				addr0: 100,
			},
			target:        addr0,
			blockNumber:   LatestBlockNumber,
			succeeded:     true,
			expectedNonce: argUintPtr(100),
		},
		{
			name: "should return zero for non-existing account",
			initialNonces: map[types.Address]uint64{
				addr0: 100,
			},
			target:        addr1,
			blockNumber:   LatestBlockNumber,
			succeeded:     true,
			expectedNonce: argUintPtr(0),
		},
		{
			name: "should return error for non-existing header",
			initialNonces: map[types.Address]uint64{
				addr0: 100,
			},
			target:        addr0,
			blockNumber:   100,
			succeeded:     false,
			expectedNonce: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &mockAccountStore{}

			for addr, nonce := range tt.initialNonces {
				account := store.AddAccount(addr)
				account.Nonce(nonce)
			}
			dispatcher := newTestDispatcher(hclog.NewNullLogger(), store)

			nonce, err := dispatcher.endpoints.Eth.GetTransactionCount(tt.target, tt.blockNumber)
			if tt.succeeded {
				assert.NoError(t, err)
				assert.NotNil(t, nonce)
				assert.Equal(t, tt.expectedNonce, nonce)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

var (
	code0 = []byte{0x1, 0x2, 0x3}
)

func TestEth_State_GetCode(t *testing.T) {
	store := &mockAccountStore{}

	acct0 := store.AddAccount(addr0)
	acct0.Code(code0)

	dispatcher := newTestDispatcher(hclog.NewNullLogger(), store)

	t.Run("Initialized address", func(t *testing.T) {
		code, err := dispatcher.endpoints.Eth.GetCode(acct0.address, LatestBlockNumber)
		assert.NoError(t, err)
		assert.Equal(t, code, argBytesPtr(code0))
	})

	t.Run("Uninitialized address", func(t *testing.T) {
		code, err := dispatcher.endpoints.Eth.GetCode(uninitializedAddress, LatestBlockNumber)
		assert.NoError(t, err)
		assert.Equal(t, code, "0x")
	})
}

func TestEth_State_GetStorageAt(t *testing.T) {
	tests := []struct {
		name           string
		initialStorage map[types.Address]map[types.Hash]types.Hash
		address        types.Address
		index          types.Hash
		blockNumber    BlockNumber
		succeeded      bool
		expectedData   *argBytes
	}{
		{
			name: "should return data for existing slot",
			initialStorage: map[types.Address]map[types.Hash]types.Hash{
				addr0: {
					hash1: hash1,
				},
			},
			address:      addr0,
			index:        hash1,
			blockNumber:  LatestBlockNumber,
			succeeded:    true,
			expectedData: argBytesPtr([]byte(hash1[:])),
		},
		{
			name: "should return 32 bytes filled with zero for undefined slot",
			initialStorage: map[types.Address]map[types.Hash]types.Hash{
				addr0: {
					hash1: hash1,
				},
			},
			address:      addr0,
			index:        hash2,
			blockNumber:  LatestBlockNumber,
			succeeded:    true,
			expectedData: argBytesPtr(types.ZeroHash[:]),
		},
		{
			name: "should return 32 bytes filled with zero for non-existing account",
			initialStorage: map[types.Address]map[types.Hash]types.Hash{
				addr0: {
					hash1: hash1,
				},
			},
			address:      addr0,
			index:        hash2,
			blockNumber:  LatestBlockNumber,
			succeeded:    true,
			expectedData: argBytesPtr(types.ZeroHash[:]),
		},
		{
			name: "should return error for non-existing header",
			initialStorage: map[types.Address]map[types.Hash]types.Hash{
				addr0: {
					hash1: hash1,
				},
			},
			address:      addr0,
			index:        hash2,
			blockNumber:  100,
			succeeded:    false,
			expectedData: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &mockAccountStore{}
			for addr, storage := range tt.initialStorage {
				account := store.AddAccount(addr)
				for index, data := range storage {
					account.Storage(index, data)
				}
			}
			dispatcher := newTestDispatcher(hclog.NewNullLogger(), store)
			res, err := dispatcher.endpoints.Eth.GetStorageAt(tt.address, tt.index, tt.blockNumber)
			if tt.succeeded {
				assert.NoError(t, err)
				assert.NotNil(t, res)
				assert.Equal(t, tt.expectedData, res)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

type mockStoreTxn struct {
	nullBlockchainInterface

	txn *types.Transaction
}

func (m *mockStoreTxn) AddTx(tx *types.Transaction) error {
	m.txn = tx
	return nil
}

func (m *mockStoreTxn) GetNonce(addr types.Address) (uint64, bool) {
	return 1, false
}

func TestEth_TxnPool_SendRawTransaction(t *testing.T) {
	store := &mockStoreTxn{}
	dispatcher := newTestDispatcher(hclog.NewNullLogger(), store)

	txn := &types.Transaction{
		From: addr0,
		V:    []byte{1},
	}
	txn.ComputeHash()

	data := txn.MarshalRLP()
	_, err := dispatcher.endpoints.Eth.SendRawTransaction(hex.EncodeToHex(data))
	assert.NoError(t, err)
	assert.NotEqual(t, store.txn.Hash, types.ZeroHash)

	// the hash in the txn pool should match the one we send
	if txn.Hash != store.txn.Hash {
		t.Fatal("bad")
	}
}

func TestEth_TxnPool_SendTransaction(t *testing.T) {
	store := &mockStoreTxn{}
	dispatcher := newTestDispatcher(hclog.NewNullLogger(), store)

	arg := &txnArgs{
		From:     argAddrPtr(addr0),
		To:       argAddrPtr(addr0),
		Nonce:    argUintPtr(0),
		GasPrice: argBytesPtr([]byte{0x1}),
	}
	_, err := dispatcher.endpoints.Eth.SendTransaction(arg)
	assert.NoError(t, err)
	assert.NotEqual(t, store.txn.Hash, types.ZeroHash)
}
