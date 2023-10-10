package jsonrpc

import (
	"errors"
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/blockchain"
	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/helper/progress"
	"github.com/0xPolygon/polygon-edge/state/runtime"
	"github.com/0xPolygon/polygon-edge/txpool/proto"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
)

func TestEth_Block_GetBlockByNumber(t *testing.T) {
	store := &mockBlockStore{}
	for i := 0; i < 10; i++ {
		store.add(newTestBlock(uint64(i), hash1))
	}

	eth := newTestEthEndpoint(store)

	cases := []struct {
		description string
		blockNum    BlockNumber
		isNotNil    bool
		err         bool
	}{
		{"should be able to get the latest block number", LatestBlockNumber, true, false},
		{"should be able to get the earliest block number", EarliestBlockNumber, true, false},
		{"should not be able to get block with negative number", BlockNumber(-50), false, true},
		{"should be able to get block with number 0", BlockNumber(0), true, false},
		{"should be able to get block with number 2", BlockNumber(2), true, false},
		{"should be able to get block with number greater than latest block", BlockNumber(50), false, false},
	}
	for _, c := range cases {
		res, err := eth.GetBlockByNumber(c.blockNum, false)

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
	store := &mockBlockStore{}
	store.add(newTestBlock(1, hash1))

	eth := newTestEthEndpoint(store)

	res, err := eth.GetBlockByHash(hash1, false)
	assert.NoError(t, err)
	assert.NotNil(t, res)

	res, err = eth.GetBlockByHash(hash2, false)
	assert.NoError(t, err)
	assert.Nil(t, res)
}

func TestEth_Block_BlockNumber(t *testing.T) {
	store := &mockBlockStore{}
	store.add(&types.Block{
		Header: &types.Header{
			Number: 10,
		},
	})

	eth := newTestEthEndpoint(store)

	num, err := eth.BlockNumber()
	assert.NoError(t, err)
	assert.Equal(t, argUintPtr(10), num)
}

func TestEth_Block_GetBlockTransactionCountByNumber(t *testing.T) {
	store := &mockBlockStore{}
	block := newTestBlock(1, hash1)

	for i := 0; i < 10; i++ {
		block.Transactions = append(block.Transactions, []*types.Transaction{{Nonce: 0, From: addr0}}...)
	}
	store.add(block)

	eth := newTestEthEndpoint(store)

	res, err := eth.GetBlockTransactionCountByNumber(BlockNumber(block.Header.Number))

	assert.NoError(t, err)
	assert.NotNil(t, res, "expected to return block, but got nil")
	assert.Equal(t, "0xa", res)
}

func TestEth_GetTransactionByHash(t *testing.T) {
	t.Parallel()

	t.Run("returns correct transaction data if transaction is found in a sealed block", func(t *testing.T) {
		t.Parallel()

		store := &mockBlockStore{}
		eth := newTestEthEndpoint(store)
		block := newTestBlock(1, hash1)
		store.add(block)

		for i := 0; i < 10; i++ {
			txn := newTestTransaction(uint64(i), addr0)
			block.Transactions = append(block.Transactions, txn)
		}

		testTxnIndex := 5
		testTxn := block.Transactions[testTxnIndex]

		res, err := eth.GetTransactionByHash(testTxn.Hash)
		assert.NoError(t, err)
		assert.NotNil(t, res)

		//nolint:forcetypeassert
		foundTxn := res.(*transaction)
		assert.Equal(t, argUint64(testTxn.Nonce), foundTxn.Nonce)
		assert.Equal(t, argUint64(block.Number()), *foundTxn.BlockNumber)
		assert.Equal(t, block.Hash(), *foundTxn.BlockHash)
		assert.Equal(t, argUint64(testTxnIndex), *foundTxn.TxIndex)
	})

	t.Run("returns correct transaction data if transaction is found in tx pool (pending)", func(t *testing.T) {
		t.Parallel()

		store := &mockBlockStore{}
		eth := newTestEthEndpoint(store)

		for i := 0; i < 10; i++ {
			txn := newTestTransaction(uint64(i), addr0)
			store.pendingTxns = append(store.pendingTxns, txn)
		}

		testTxn := store.pendingTxns[5]

		res, err := eth.GetTransactionByHash(testTxn.Hash)
		assert.NoError(t, err)
		assert.NotNil(t, res)

		//nolint:forcetypeassert
		foundTxn := res.(*transaction)
		assert.Equal(t, argUint64(testTxn.Nonce), foundTxn.Nonce)
		assert.Nil(t, foundTxn.BlockNumber)
		assert.Nil(t, foundTxn.BlockHash)
		assert.Nil(t, foundTxn.TxIndex)
	})

	t.Run("returns nil if transaction is nowhere to be found", func(t *testing.T) {
		t.Parallel()

		eth := newTestEthEndpoint(&mockBlockStore{})

		res, err := eth.GetTransactionByHash(types.StringToHash("abcdef"))

		assert.NoError(t, err)
		assert.Nil(t, res)
	})
}

func TestEth_GetTransactionReceipt(t *testing.T) {
	t.Parallel()

	t.Run("returns nil if transaction with same hash not found", func(t *testing.T) {
		t.Parallel()

		store := &mockBlockStore{}
		eth := newTestEthEndpoint(store)

		res, err := eth.GetTransactionReceipt(hash1)

		assert.NoError(t, err)
		assert.Nil(t, res)
	})

	t.Run("returns correct receipt data for found transaction", func(t *testing.T) {
		t.Parallel()

		store := newMockBlockStore()
		eth := newTestEthEndpoint(store)
		block := newTestBlock(1, hash4)
		store.add(block)
		txn0 := newTestTransaction(uint64(0), addr0)
		txn1 := newTestTransaction(uint64(1), addr1)
		block.Transactions = []*types.Transaction{txn0, txn1}
		receipt1 := &types.Receipt{
			Logs: []*types.Log{
				{
					// log 0
					Topics: []types.Hash{
						hash1,
					},
				},
				{
					// log 1
					Topics: []types.Hash{
						hash2,
					},
				},
				{
					// log 2
					Topics: []types.Hash{
						hash3,
					},
				},
			},
		}
		receipt1.SetStatus(types.ReceiptSuccess)
		receipt2 := &types.Receipt{
			Logs: []*types.Log{
				{
					// log 3
					Topics: []types.Hash{
						hash4,
					},
				},
			},
		}
		receipt2.SetStatus(types.ReceiptSuccess)
		store.receipts[hash4] = []*types.Receipt{receipt1, receipt2}

		res, err := eth.GetTransactionReceipt(txn1.Hash)

		assert.NoError(t, err)
		assert.NotNil(t, res)

		//nolint:forcetypeassert
		response := res.(*receipt)
		assert.Equal(t, txn1.Hash, response.TxHash)
		assert.Equal(t, block.Hash(), response.BlockHash)
		assert.NotNil(t, response.Logs)
		assert.Len(t, response.Logs, 1)
		assert.Equal(t, uint64(3), uint64(response.Logs[0].LogIndex))
		assert.Equal(t, uint64(1), uint64(response.Logs[0].TxIndex))
	})
}

func TestEth_Syncing(t *testing.T) {
	store := newMockBlockStore()
	eth := newTestEthEndpoint(store)

	t.Run("returns progression struct if sync is progress", func(t *testing.T) {
		store.isSyncing = true

		res, err := eth.Syncing()

		assert.NoError(t, err)
		assert.NotNil(t, res)

		//nolint:forcetypeassert
		response := res.(progression)
		assert.NotEqual(t, progress.ChainSyncBulk, response.Type)
		assert.Equal(t, argUint64(1), response.StartingBlock)
		assert.Equal(t, argUint64(10), response.CurrentBlock)
		assert.Equal(t, argUint64(100), response.HighestBlock)
	})

	t.Run("returns \"false\" if sync is not progress", func(t *testing.T) {
		store.isSyncing = false

		res, err := eth.Syncing()

		assert.NoError(t, err)
		//nolint:forcetypeassert
		assert.False(t, res.(bool))
	})
}

func TestEth_GasPrice_WithLondonFork(t *testing.T) {
	const (
		baseFee    = uint64(10000)
		tipCap     = uint64(1000)
		priceLimit = uint64(10010)
	)

	store := newMockBlockStore()
	store.blocks = []*types.Block{
		{
			Header: &types.Header{Number: uint64(1)},
		},
	}
	store.maxPriorityFeePerGasFn = func() (*big.Int, error) {
		return new(big.Int).SetUint64(tipCap), nil
	}

	// not using newTestEthEndpoint as we need to set priceLimit
	eth := newTestEthEndpointWithPriceLimit(store, priceLimit)

	t.Run("returns price limit flag value when it is larger than MaxPriorityFee+BaseFee", func(t *testing.T) {
		store.baseFee = 0

		res, err := eth.GasPrice()

		assert.NoError(t, err)
		assert.NotNil(t, res)
		assert.Equal(t, argUint64(priceLimit), res)
	})

	t.Run("returns MaxPriorityFee+BaseFee when it is larger than set price limit flag", func(t *testing.T) {
		store.baseFee = baseFee

		res, err := eth.GasPrice()

		assert.NoError(t, err)
		assert.NotNil(t, res)
		assert.Equal(t, argUint64(baseFee+tipCap), res)
	})

	t.Run("returns error if MaxPriorityFeePerGas returns error", func(t *testing.T) {
		store.maxPriorityFeePerGasFn = func() (*big.Int, error) {
			return nil, runtime.ErrDepth
		}

		_, err := eth.GasPrice()

		assert.ErrorIs(t, err, runtime.ErrDepth)
	})
}

func TestEth_GasPrice_WithoutLondonFork(t *testing.T) {
	const priceLimit = 100000

	store := newMockBlockStore()
	store.forksInTime.London = false
	store.blocks = []*types.Block{
		{
			Header: &types.Header{Number: uint64(1)},
		},
	}

	eth := newTestEthEndpointWithPriceLimit(store, priceLimit)

	t.Run("priceLimit is greater than averageGasPrice", func(t *testing.T) {
		store.averageGasPrice = priceLimit - 100

		res, err := eth.GasPrice()

		assert.NoError(t, err)
		assert.NotNil(t, res)
		assert.Equal(t, argUint64(priceLimit), res)
	})

	t.Run("averageGasPrice is greater than priceLimit", func(t *testing.T) {
		store.averageGasPrice = priceLimit + 100

		res, err := eth.GasPrice()

		assert.NoError(t, err)
		assert.NotNil(t, res)
		assert.Equal(t, argUint64(priceLimit+100), res)
	})
}

func TestEth_Call(t *testing.T) {
	t.Parallel()

	t.Run("returns error if transaction execution fails", func(t *testing.T) {
		t.Parallel()

		store := newMockBlockStore()
		store.add(newTestBlock(100, hash1))
		store.ethCallError = errors.New("an arbitrary error")
		eth := newTestEthEndpoint(store)
		contractCall := &txnArgs{
			From:     &addr0,
			To:       &addr1,
			Gas:      argUintPtr(100000),
			GasPrice: argBytesPtr([]byte{0x64}),
			Value:    argBytesPtr([]byte{0x64}),
			Data:     nil,
			Nonce:    argUintPtr(0),
		}

		res, err := eth.Call(contractCall, BlockNumberOrHash{}, nil)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), store.ethCallError.Error())
		assert.Nil(t, res)
	})

	t.Run("returns a value representing result of the successful transaction execution", func(t *testing.T) {
		t.Parallel()

		store := newMockBlockStore()
		store.add(newTestBlock(100, hash1))
		store.ethCallError = nil
		eth := newTestEthEndpoint(store)
		contractCall := &txnArgs{
			From:     &addr0,
			To:       &addr1,
			Gas:      argUintPtr(100000),
			GasPrice: argBytesPtr([]byte{0x64}),
			Value:    argBytesPtr([]byte{0x64}),
			Data:     nil,
			Nonce:    argUintPtr(0),
		}

		res, err := eth.Call(contractCall, BlockNumberOrHash{}, nil)

		assert.NoError(t, err)
		assert.NotNil(t, res)
	})

	t.Run("returns error and result as data of a reverted transaction execution", func(t *testing.T) {
		t.Parallel()

		returnValue := []byte("Reverted()")

		store := newMockBlockStore()
		store.add(newTestBlock(100, hash1))
		store.ethCallError = runtime.ErrExecutionReverted
		store.returnValue = returnValue
		eth := newTestEthEndpoint(store)
		contractCall := &txnArgs{
			From:     &addr0,
			To:       &addr1,
			Gas:      argUintPtr(100000),
			GasPrice: argBytesPtr([]byte{0x64}),
			Value:    argBytesPtr([]byte{0x64}),
			Data:     nil,
			Nonce:    argUintPtr(0),
		}

		res, err := eth.Call(contractCall, BlockNumberOrHash{}, nil)
		assert.Error(t, err)
		assert.NotNil(t, res)
		bres := res.([]byte) //nolint:forcetypeassert
		assert.Equal(t, []byte(hex.EncodeToString(returnValue)), bres)
	})
}

type testStore interface {
	ethStore
}

type mockBlockStore struct {
	testStore
	blocks          []*types.Block
	topics          []types.Hash
	pendingTxns     []*types.Transaction
	receipts        map[types.Hash][]*types.Receipt
	isSyncing       bool
	averageGasPrice int64
	ethCallError    error
	returnValue     []byte
	forksInTime     chain.ForksInTime
	baseFee         uint64

	maxPriorityFeePerGasFn func() (*big.Int, error)
}

func newMockBlockStore() *mockBlockStore {
	store := &mockBlockStore{}
	store.receipts = make(map[types.Hash][]*types.Receipt)
	store.forksInTime = chain.AllForksEnabled.At(0)

	return store
}

func (m *mockBlockStore) add(blocks ...*types.Block) {
	if m.blocks == nil {
		m.blocks = []*types.Block{}
	}

	m.blocks = append(m.blocks, blocks...)
}

func (m *mockBlockStore) appendBlocksToStore(blocks []*types.Block) {
	if m.blocks == nil {
		m.blocks = []*types.Block{}
	}

	for _, block := range blocks {
		if block == nil {
			continue
		}

		m.blocks = append(m.blocks, block)
	}
}

func (m *mockBlockStore) setupLogs() {
	m.receipts = make(map[types.Hash][]*types.Receipt)

	m.receipts[hash1] = []*types.Receipt{
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
	}

	m.receipts[hash2] = []*types.Receipt{
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
	}

	m.receipts[hash3] = []*types.Receipt{
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
	}
}

func (m *mockBlockStore) GetReceiptsByHash(hash types.Hash) ([]*types.Receipt, error) {
	receipts, ok := m.receipts[hash]
	if !ok {
		return nil, nil
	}

	return receipts, nil
}

func (m *mockBlockStore) GetBlockByNumber(blockNumber uint64, full bool) (*types.Block, bool) {
	for _, b := range m.blocks {
		if b.Number() == blockNumber {
			return b, true
		}
	}

	return nil, false
}

func (m *mockBlockStore) GetBlockByHash(hash types.Hash, full bool) (*types.Block, bool) {
	for _, b := range m.blocks {
		if b.Hash() == hash {
			return b, true
		}
	}

	return nil, false
}

func (m *mockBlockStore) Header() *types.Header {
	return m.blocks[len(m.blocks)-1].Header
}

func (m *mockBlockStore) ReadTxLookup(txnHash types.Hash) (types.Hash, bool) {
	for _, block := range m.blocks {
		for _, txn := range block.Transactions {
			if txn.Hash == txnHash {
				return block.Hash(), true
			}
		}
	}

	return types.ZeroHash, false
}

func (m *mockBlockStore) GetPendingTx(txHash types.Hash) (*types.Transaction, bool) {
	for _, txn := range m.pendingTxns {
		if txn.Hash == txHash {
			return txn, true
		}
	}

	return nil, false
}

func (m *mockBlockStore) GetSyncProgression() *progress.Progression {
	if m.isSyncing {
		return &progress.Progression{
			SyncType:      progress.ChainSyncBulk,
			StartingBlock: 1,
			CurrentBlock:  10,
			HighestBlock:  100,
		}
	} else {
		return nil
	}
}

func (m *mockBlockStore) GetAvgGasPrice() *big.Int {
	return big.NewInt(m.averageGasPrice)
}

func (m *mockBlockStore) ApplyTxn(_ *types.Header, _ *types.Transaction, _ types.StateOverride, _ bool) (*runtime.ExecutionResult, error) {
	return &runtime.ExecutionResult{
		Err:         m.ethCallError,
		ReturnValue: m.returnValue,
	}, nil
}

func (m *mockBlockStore) SubscribeEvents() blockchain.Subscription {
	return nil
}

func (m *mockBlockStore) FilterExtra(extra []byte) ([]byte, error) {
	return extra, nil
}

func (m *mockBlockStore) TxPoolSubscribe(request *proto.SubscribeRequest) (<-chan *proto.TxPoolEvent, func(), error) {
	return nil, nil, nil
}

func (m *mockBlockStore) GetAccount(root types.Hash, addr types.Address) (*Account, error) {
	return &Account{Nonce: 0}, nil
}

func (m *mockBlockStore) GetBaseFee() uint64 {
	return m.baseFee
}

func (m *mockBlockStore) GetForksInTime(block uint64) chain.ForksInTime {
	return m.forksInTime
}

func (m *mockBlockStore) MaxPriorityFeePerGas() (*big.Int, error) {
	if m.maxPriorityFeePerGasFn != nil {
		return m.maxPriorityFeePerGasFn()
	}

	return big.NewInt(0), nil
}

func newTestBlock(number uint64, hash types.Hash) *types.Block {
	return &types.Block{
		Header: &types.Header{
			Number: number,
			Hash:   hash,
		},
	}
}
