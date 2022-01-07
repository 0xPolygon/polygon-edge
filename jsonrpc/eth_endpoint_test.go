package jsonrpc

import (
	"bytes"
	"fmt"
	"math/big"
	"strconv"
	"testing"

	"github.com/0xPolygon/polygon-sdk/chain"
	"github.com/0xPolygon/polygon-sdk/helper/hex"
	"github.com/0xPolygon/polygon-sdk/state"
	"github.com/0xPolygon/polygon-sdk/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
)

type mockAccount2 struct {
	store   *mockAccountStore
	address types.Address
	code    []byte
	account *state.Account
	storage map[types.Hash][]byte
}

func (m *mockAccount2) Storage(k types.Hash, v []byte) {
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

func (m *mockAccountStore) GetForksInTime(blockNumber uint64) chain.ForksInTime {
	panic("implement me")
}

func (m *mockAccountStore) GetCapacity() (uint64, uint64) {
	panic("implement me")
}

func (m *mockAccountStore) AddAccount(addr types.Address) *mockAccount2 {
	if m.accounts == nil {
		m.accounts = map[types.Address]*mockAccount2{}
	}
	acct := &mockAccount2{
		store:   m,
		address: addr,
		account: &state.Account{},
		storage: make(map[types.Hash][]byte),
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
	return val, nil
}

type mockBlockStore2 struct {
	nullBlockchainInterface
	blocks []*types.Block
	topics []types.Hash
}

func (m *mockBlockStore2) GetForksInTime(blockNumber uint64) chain.ForksInTime {
	panic("implement me")
}

func (m *mockBlockStore2) GetCapacity() (uint64, uint64) {
	panic("implement me")
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

func TestEth_Block_GetBlockTransactionCountByNumber(t *testing.T) {
	store := &mockBlockStore2{}
	for i := 0; i < 10; i++ {
		store.add(&types.Block{
			Header: &types.Header{
				Number: uint64(i),
			},
			Transactions: []*types.Transaction{{From: addr0}},
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
		res, err := dispatcher.endpoints.Eth.GetBlockTransactionCountByNumber(c.blockNum)

		if c.isNotNil {
			assert.NotNil(t, res, "expected to return block, but got nil")
			assert.Equal(t, res, 1)
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

func TestEth_Block_GetLogs(t *testing.T) {

	blockHash := types.StringToHash("1")

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
		{"Found matching logs, BlockHash present",
			&LogFilter{
				BlockHash: &blockHash,
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
	for i := 0; i < 5; i++ {
		store.add(&types.Block{
			Header: &types.Header{
				Number: uint64(i),
				Hash:   types.StringToHash(strconv.Itoa(i)),
			},
			Transactions: []*types.Transaction{
				{
					Value: big.NewInt(10),
				},
				{
					Value: big.NewInt(11),
				},
				{
					Value: big.NewInt(12),
				},
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
				assert.Lenf(t, foundLogs, testCase.expectedLength, "Invalid number of logs found")
			} else {
				assert.Nil(t, foundLogs, "Expected first return param to be nil")
			}
		})
	}
}

var (
	addr0                = types.Address{0x1}
	uninitializedAddress = types.Address{0x99}
)

type mockSpecialStore struct {
	nullBlockchainInterface
	account *mockAccount2
	block   *types.Block
}

func (m *mockSpecialStore) GetForksInTime(blockNumber uint64) chain.ForksInTime {
	//TODO implement me
	panic("implement me")
}

func (m *mockSpecialStore) GetBlockByHash(hash types.Hash, full bool) (*types.Block, bool) {
	if m.block.Header.Hash.String() != hash.String() {
		return nil, false
	}

	return m.block, true
}

func (m *mockSpecialStore) GetAccount(root types.Hash, addr types.Address) (*state.Account, error) {
	if m.account.address.String() != addr.String() {
		return nil, ErrStateNotFound
	}
	return m.account.account, nil
}

func (m *mockSpecialStore) GetHeaderByNumber(blockNumber uint64) (*types.Header, bool) {
	if m.block.Number() != blockNumber {
		return nil, false
	}
	return m.block.Header, true
}

func (m *mockSpecialStore) Header() *types.Header {
	return &types.Header{
		StateRoot: types.EmptyRootHash,
	}
}

func (m *mockSpecialStore) GetNonce(addr types.Address) uint64 {
	return 1
}

func (m *mockSpecialStore) GetCode(hash types.Hash) ([]byte, error) {
	if bytes.Equal(m.account.account.CodeHash, hash.Bytes()) {
		return m.account.code, nil
	}
	return nil, fmt.Errorf("code not found")
}

func TestEth_State_GetBalance(t *testing.T) {
	store := &mockSpecialStore{
		account: &mockAccount2{
			address: addr0,
			account: &state.Account{
				Balance: big.NewInt(100),
			},
			storage: make(map[types.Hash][]byte),
		},
		block: &types.Block{
			Header: &types.Header{
				Hash:      types.ZeroHash,
				Number:    0,
				StateRoot: types.EmptyRootHash,
			},
		},
	}

	dispatcher := newTestDispatcher(hclog.NewNullLogger(), store)
	blockNumberLatest := LatestBlockNumber
	blockNumberZero := BlockNumber(0x0)
	blockNumberInvalid := BlockNumber(0x1)

	tests := []struct {
		name            string
		address         types.Address
		shouldFail      bool
		blockNumber     *BlockNumber
		blockHash       *types.Hash
		expectedBalance int64
	}{
		{
			"valid implicit latest block number",
			addr0,
			false,
			nil,
			nil,
			100,
		},
		{
			"explicit latest block number",
			addr0,
			false,
			&blockNumberLatest,
			nil,
			100,
		},
		{
			"valid explicit block number",
			addr0,
			false,
			&blockNumberZero,
			nil,
			100,
		},
		{
			"block does not exist",
			addr0,
			true,
			&blockNumberInvalid,
			nil,
			0,
		},
		{
			"valid block hash",
			addr0,
			false,
			nil,
			&types.ZeroHash,
			100,
		},
		{
			"invalid block hash",
			addr0,
			true,
			nil,
			&hash1,
			0,
		},
		{
			"account with empty balance",
			addr1,
			false,
			&blockNumberLatest,
			nil,
			0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := BlockNumberOrHash{
				BlockNumber: tt.blockNumber,
				BlockHash:   tt.blockHash,
			}

			balance, err := dispatcher.endpoints.Eth.GetBalance(tt.address, filter)

			if tt.shouldFail {
				assert.Error(t, err)
				assert.Equal(t, nil, balance)
			} else {
				assert.NoError(t, err)
				if tt.expectedBalance == 0 {
					assert.Equal(t, *argUintPtr(0), *balance.(*argUint64))
				} else {
					assert.Equal(t, *argBigPtr(big.NewInt(tt.expectedBalance)), *balance.(*argBig))
				}
			}
		})
	}
}

func TestEth_State_GetTransactionCount(t *testing.T) {
	store := &mockSpecialStore{
		account: &mockAccount2{
			address: addr0,
			account: &state.Account{
				Balance: big.NewInt(100),
				Nonce:   100,
			},
			storage: make(map[types.Hash][]byte),
		},
		block: &types.Block{
			Header: &types.Header{
				Hash:      types.ZeroHash,
				Number:    0,
				StateRoot: types.EmptyRootHash,
			},
		},
	}

	dispatcher := newTestDispatcher(hclog.NewNullLogger(), store)
	blockNumberLatest := LatestBlockNumber
	blockNumberZero := BlockNumber(0x0)
	blockNumberInvalid := BlockNumber(0x1)

	tests := []struct {
		name          string
		target        types.Address
		blockNumber   *BlockNumber
		blockHash     *types.Hash
		shouldFail    bool
		expectedNonce uint64
	}{
		{
			"should return valid nonce for implicit block number",
			addr0,
			nil,
			nil,
			false,
			100,
		},
		{
			"should return valid nonce for explicit latest block number",
			addr0,
			&blockNumberLatest,
			nil,
			false,
			100,
		},
		{
			"should return valid nonce for explicit block number",
			addr0,
			&blockNumberZero,
			nil,
			false,
			100,
		},
		{
			"should return an error for non-existing block",
			addr0,
			&blockNumberInvalid,
			nil,
			true,
			0,
		},
		{
			"should return valid nonce for valid block hash",
			addr0,
			nil,
			&types.ZeroHash,
			false,
			100,
		},
		{
			"should return an error for invalid block hash",
			addr0,
			nil,
			&hash1,
			true,
			0,
		},
		{
			"should return a zero-nonce for non-existing account",
			addr1,
			&blockNumberLatest,
			nil,
			false,
			0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := BlockNumberOrHash{
				BlockNumber: tt.blockNumber,
				BlockHash:   tt.blockHash,
			}

			nonce, err := dispatcher.endpoints.Eth.GetTransactionCount(tt.target, filter)

			if tt.shouldFail {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, argUintPtr(tt.expectedNonce), nonce)
			}
		})
	}
}

var (
	code0 = []byte{0x1, 0x2, 0x3}
)

func TestEth_State_GetCode(t *testing.T) {
	store := &mockSpecialStore{
		account: &mockAccount2{
			address: addr0,
			account: &state.Account{
				Balance:  big.NewInt(100),
				Nonce:    100,
				CodeHash: types.BytesToHash(addr0.Bytes()).Bytes(),
			},
			code: code0,
		},
		block: &types.Block{
			Header: &types.Header{
				Hash:      types.ZeroHash,
				Number:    0,
				StateRoot: types.EmptyRootHash,
			},
		},
	}

	dispatcher := newTestDispatcher(hclog.NewNullLogger(), store)
	blockNumberLatest := LatestBlockNumber
	blockNumberZero := BlockNumber(0x0)
	blockNumberInvalid := BlockNumber(0x1)

	emptyCode := []byte("0x")

	tests := []struct {
		name         string
		target       types.Address
		blockNumber  *BlockNumber
		blockHash    *types.Hash
		shouldFail   bool
		expectedCode []byte
	}{
		{
			"should return a valid code for implicit block number",
			addr0,
			nil,
			nil,
			false,
			code0,
		},
		{
			"should return a valid code for explicit latest block number",
			addr0,
			&blockNumberLatest,
			nil,
			false,
			code0,
		},
		{
			"should return a valid code for explicit block number",
			addr0,
			&blockNumberZero,
			nil,
			false,
			code0,
		},
		{
			"should return an error for non-existing block",
			addr0,
			&blockNumberInvalid,
			nil,
			true,
			emptyCode,
		},
		{
			"should return a valid code for valid block hash",
			addr0,
			nil,
			&types.ZeroHash,
			false,
			code0,
		},
		{
			"should return an error for invalid block hash",
			addr0,
			nil,
			&hash1,
			true,
			emptyCode,
		},
		{
			"should not return an error for non-existing account",
			uninitializedAddress,
			&blockNumberLatest,
			nil,
			false,
			emptyCode,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := BlockNumberOrHash{
				BlockNumber: tt.blockNumber,
				BlockHash:   tt.blockHash,
			}

			code, err := dispatcher.endpoints.Eth.GetCode(tt.target, filter)

			if tt.shouldFail {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.target.String() == uninitializedAddress.String() {
					assert.Equal(t, "0x", code)
				} else {
					assert.Equal(t, argBytesPtr(tt.expectedCode), code)
				}
			}
		})
	}
}

func TestEth_State_GetStorageAt(t *testing.T) {
	/*
		latestBlock := LatestBlockNumber
		blockAt64 := BlockNumber(0x64)
		filter := BlockNumberOrHash{
			BlockNumber: &latestBlock,
		}

		tests := []struct {
			name           string
			initialStorage map[types.Address]map[types.Hash]types.Hash
			address        types.Address
			index          types.Hash
			blockNumber    BlockNumberOrHash
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
				blockNumber:  filter,
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
				blockNumber:  filter,
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
				blockNumber:  filter,
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
				blockNumber:  BlockNumberOrHash{BlockNumber: &blockAt64},
				succeeded:    false,
				expectedData: nil,
			},
			{
				name: "should not return error for empty block parameter",
				initialStorage: map[types.Address]map[types.Hash]types.Hash{
					addr0: {
						hash1: hash1,
					},
				},
				address:      addr0,
				index:        hash2,
				blockNumber:  BlockNumberOrHash{},
				succeeded:    true,
				expectedData: argBytesPtr(types.ZeroHash[:]),
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				store := &mockAccountStore{}
				for addr, storage := range tt.initialStorage {
					account := store.AddAccount(addr)
					for index, data := range storage {
						a := &fastrlp.Arena{}
						value := a.NewBytes(data.Bytes())
						newData := value.MarshalTo(nil)
						account.Storage(index, newData)
					}
				}
				dispatcher := newTestDispatcher(hclog.NewNullLogger(), store)
				// blockNumber, _ := createBlockNumberPointer(tt.blockNumber)
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
	*/
}

type mockStoreTxn struct {
	nullBlockchainInterface
	accounts map[types.Address]*mockAccount2
	txn      *types.Transaction
}

func (m *mockStoreTxn) GetForksInTime(blockNumber uint64) chain.ForksInTime {
	panic("implement me")
}

func (m *mockStoreTxn) AddTx(tx *types.Transaction) error {
	m.txn = tx
	return nil
}

func (m *mockStoreTxn) GetNonce(addr types.Address) uint64 {
	return 1
}
func (m *mockStoreTxn) AddAccount(addr types.Address) *mockAccount2 {
	if m.accounts == nil {
		m.accounts = map[types.Address]*mockAccount2{}
	}
	acct := &mockAccount2{
		address: addr,
		account: &state.Account{},
		storage: make(map[types.Hash][]byte),
	}
	m.accounts[addr] = acct
	return acct
}

func (m *mockStoreTxn) Header() *types.Header {
	return &types.Header{}
}

func (m *mockStoreTxn) GetAccount(root types.Hash, addr types.Address) (*state.Account, error) {
	acct, ok := m.accounts[addr]
	if !ok {
		return nil, ErrStateNotFound
	}
	return acct.account, nil
}
func TestEth_TxnPool_SendRawTransaction(t *testing.T) {
	store := &mockStoreTxn{}
	dispatcher := newTestDispatcher(hclog.NewNullLogger(), store)

	txn := &types.Transaction{
		From: addr0,
		V:    big.NewInt(1),
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
	store.AddAccount(addr0)
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
