package jsonrpc

import (
	"bytes"
	"fmt"
	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/state/runtime"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
	"github.com/umbracle/fastrlp"
	"math/big"
	"testing"
)

var (
	addr0                = types.Address{0x1}
	uninitializedAddress = types.Address{0x99}
	code0                = []byte{0x1, 0x2, 0x3}
)

func TestEth_State_GetBalance(t *testing.T) {
	store := &mockSpecialStore{
		account: &mockAccount{
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

	eth := newTestEthEndpoint(store)
	blockNumberEarliest := EarliestBlockNumber
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
			"should return the balance based on the earliest block",
			addr0,
			false,
			&blockNumberEarliest,
			nil,
			100,
		},
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

			balance, err := eth.GetBalance(tt.address, filter)

			if tt.shouldFail {
				assert.Error(t, err)
				assert.Equal(t, nil, balance)
			} else {
				assert.NoError(t, err)
				if tt.expectedBalance == 0 {
					uintBalance, ok := balance.(*argUint64)
					if !ok {
						t.Fatalf("invalid type assertion")
					}

					assert.Equal(t, *argUintPtr(0), *uintBalance)
				} else {
					bigBalance, ok := balance.(*argBig)
					if !ok {
						t.Fatalf("invalid type assertion")
					}

					assert.Equal(t, *argBigPtr(big.NewInt(tt.expectedBalance)), *bigBalance)
				}
			}
		})
	}
}

func TestEth_State_GetTransactionCount(t *testing.T) {
	store := &mockSpecialStore{
		account: &mockAccount{
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

	eth := newTestEthEndpoint(store)
	blockNumberEarliest := EarliestBlockNumber
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
			"should return valid nonce using earliest block number",
			addr0,
			&blockNumberEarliest,
			nil,
			false,
			100,
		},
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

			nonce, err := eth.GetTransactionCount(tt.target, filter)

			if tt.shouldFail {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, argUintPtr(tt.expectedNonce), nonce)
			}
		})
	}
}

func TestEth_State_GetCode(t *testing.T) {
	store := &mockSpecialStore{
		account: &mockAccount{
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

	eth := newTestEthEndpoint(store)
	blockNumberEarliest := EarliestBlockNumber
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
			"should return a valid code using earliest block number",
			addr0,
			&blockNumberEarliest,
			nil,
			false,
			code0,
		},
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

			code, err := eth.GetCode(tt.target, filter)

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
	store := &mockSpecialStore{
		account: &mockAccount{
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

	eth := newTestEthEndpoint(store)
	blockNumberEarliest := EarliestBlockNumber
	blockNumberLatest := LatestBlockNumber
	blockNumberZero := BlockNumber(0x0)
	blockNumberInvalid := BlockNumber(0x1)

	tests := []struct {
		name           string
		initialStorage map[types.Address]map[types.Hash]types.Hash
		address        types.Address
		index          types.Hash
		blockNumber    *BlockNumber
		blockHash      *types.Hash
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
			blockNumber:  nil,
			blockHash:    nil,
			succeeded:    true,
			expectedData: argBytesPtr(hash1[:]),
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
			blockNumber:  &blockNumberLatest,
			blockHash:    nil,
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
			blockNumber:  &blockNumberLatest,
			succeeded:    true,
			expectedData: argBytesPtr(types.ZeroHash[:]),
		},
		{
			name: "should return error for invalid block number",
			initialStorage: map[types.Address]map[types.Hash]types.Hash{
				addr0: {
					hash1: hash1,
				},
			},
			address:      addr0,
			index:        hash2,
			blockNumber:  &blockNumberInvalid,
			blockHash:    nil,
			succeeded:    false,
			expectedData: nil,
		},
		{
			name: "should not return an error for block zero",
			initialStorage: map[types.Address]map[types.Hash]types.Hash{
				addr0: {
					hash1: hash1,
				},
			},
			address:      addr0,
			index:        hash1,
			blockNumber:  &blockNumberZero,
			blockHash:    nil,
			succeeded:    true,
			expectedData: argBytesPtr(hash1[:]),
		},
		{
			name: "should not return an error for valid block hash",
			initialStorage: map[types.Address]map[types.Hash]types.Hash{
				addr0: {
					hash1: hash1,
				},
			},
			address:      addr0,
			index:        hash1,
			blockNumber:  nil,
			blockHash:    &types.ZeroHash,
			succeeded:    true,
			expectedData: argBytesPtr(hash1[:]),
		},
		{
			name: "should return error for invalid block hash",
			initialStorage: map[types.Address]map[types.Hash]types.Hash{
				addr0: {
					hash1: hash1,
				},
			},
			address:      addr0,
			index:        hash2,
			blockNumber:  nil,
			blockHash:    &hash1,
			succeeded:    false,
			expectedData: nil,
		},
		{
			name: "should return data using earliest block number",
			initialStorage: map[types.Address]map[types.Hash]types.Hash{
				addr0: {
					hash1: hash1,
				},
			},
			address:      addr0,
			index:        hash1,
			blockNumber:  &blockNumberEarliest,
			blockHash:    nil,
			succeeded:    true,
			expectedData: argBytesPtr(hash1[:]),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for addr, storage := range tt.initialStorage {
				store.account = &mockAccount{
					address: addr,
					account: &state.Account{
						Balance: big.NewInt(100),
						Nonce:   100,
					},
					storage: make(map[types.Hash][]byte),
				}
				account := store.account
				for index, data := range storage {
					a := &fastrlp.Arena{}
					value := a.NewBytes(data.Bytes())
					newData := value.MarshalTo(nil)
					account.Storage(index, newData)
				}
			}

			filter := BlockNumberOrHash{
				BlockNumber: tt.blockNumber,
				BlockHash:   tt.blockHash,
			}

			res, err := eth.GetStorageAt(tt.address, tt.index, filter)
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

func TestEth_EstimateGas(t *testing.T) {
	newTxnArgs := func(gasLimit uint64) *txnArgs {
		return &txnArgs{
			From:     &addr0,
			To:       &addr1,
			Gas:      argUintPtr(gasLimit),
			GasPrice: argBytesPtr([]byte{0x1}),
			Value:    argBytesPtr([]byte{0x64}),
			Input:    argBytesPtr([]byte{0x64}),
			Data:     nil,
			Nonce:    argUintPtr(0),
		}
	}

	setupMockStore := func(accountBalance int64) *mockSpecialStore {
		return &mockSpecialStore{
			account: &mockAccount{
				address: addr0,
				account: &state.Account{
					Balance: big.NewInt(accountBalance),
					Nonce:   0,
				},
				storage: make(map[types.Hash][]byte),
			},
			block: &types.Block{
				Header: &types.Header{
					Hash:      hash1,
					Number:    0,
					StateRoot: types.EmptyRootHash,
					GasLimit:  5000000,
				},
			},
		}
	}

	t.Run(
		"returns error if account doesn't have enough funds for transaction execution",
		func(t *testing.T) {
			balance := int64(100)
			txnGasLimit := uint64(30000)
			store := setupMockStore(balance)
			eth := newTestEthEndpoint(store)
			txn := newTxnArgs(txnGasLimit)

			// The value of the transaction is greater than the balance
			txn.Value = argBytesPtr(big.NewInt(300).Bytes())

			res, err := eth.EstimateGas(txn, nil)

			assert.Error(t, err)
			assert.Contains(t, err.Error(), "insufficient funds for execution")
			assert.Nil(t, res)
		})

	t.Run(
		"returns estimated gas value +500 when transaction has gas field greater than estimated gas value",
		func(t *testing.T) {
			balance := int64(10000000)
			txnGasLimit := uint64(30000)
			store := setupMockStore(balance)
			store.estimatedGasPivotValue = 25000
			eth := newTestEthEndpoint(store)
			txn := newTxnArgs(txnGasLimit)

			res, err := eth.EstimateGas(txn, nil)

			assert.NoError(t, err)
			assert.NotNil(t, res)
			// nolint:forcetypeassert
			assert.Equal(t, fmt.Sprintf("0x%x", store.estimatedGasPivotValue+500), res.(string))
		})

	t.Run(
		"returns transaction's gas limit value +1 when estimated gas value exceeds it",
		func(t *testing.T) {
			balance := int64(10000000)
			txnGasLimit := uint64(23000)
			store := setupMockStore(balance)
			store.estimatedGasPivotValue = 25000
			eth := newTestEthEndpoint(store)
			txn := newTxnArgs(txnGasLimit)

			res, err := eth.EstimateGas(txn, nil)

			assert.NoError(t, err)
			assert.NotNil(t, res)
			// nolint:forcetypeassert
			assert.Equal(t, fmt.Sprintf("0x%x", txnGasLimit+1), res.(string))
		})
}

type mockSpecialStore struct {
	ethStore
	account                *mockAccount
	block                  *types.Block
	estimatedGasPivotValue uint64
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
	return m.block.Header
}

func (m *mockSpecialStore) GetNonce(addr types.Address) uint64 {
	return 1
}

func (m *mockSpecialStore) GetStorage(root types.Hash, addr types.Address, slot types.Hash) ([]byte, error) {
	if m.account.address.String() != addr.String() {
		return nil, ErrStateNotFound
	}

	acct := m.account
	val, ok := acct.storage[slot]

	if !ok {
		return nil, ErrStateNotFound
	}

	return val, nil
}

func (m *mockSpecialStore) GetCode(hash types.Hash) ([]byte, error) {
	if bytes.Equal(m.account.account.CodeHash, hash.Bytes()) {
		return m.account.code, nil
	}

	return nil, fmt.Errorf("code not found")
}

func (m *mockSpecialStore) GetForksInTime(blockNumber uint64) chain.ForksInTime {
	return chain.ForksInTime{}
}

func (m *mockSpecialStore) ApplyTxn(header *types.Header, txn *types.Transaction) (*runtime.ExecutionResult, error) {
	if txn.Gas <= m.estimatedGasPivotValue {
		return &runtime.ExecutionResult{Err: runtime.ErrOutOfGas}, nil
	} else {
		return &runtime.ExecutionResult{}, nil
	}
}
