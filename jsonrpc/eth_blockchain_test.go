package jsonrpc

import (
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
	"math/big"
	"strconv"
	"testing"
)

func TestEth_Block_GetBlockByNumber(t *testing.T) {
	store := &mockBlockStore{}
	for i := 0; i < 10; i++ {
		store.add(&types.Block{
			Header: &types.Header{
				Number: uint64(i),
			},
		})
	}

	eth := newTestEthEndpoint(store)

	cases := []struct {
		blockNum BlockNumber
		isNotNil bool
		err      bool
	}{
		{LatestBlockNumber, true, false},
		{EarliestBlockNumber, true, false},
		{BlockNumber(-50), false, true},
		{BlockNumber(0), true, false},
		{BlockNumber(2), true, false},
		{BlockNumber(50), false, false},
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
	store.add(&types.Block{
		Header: &types.Header{
			Hash: hash1,
		},
	})

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
	for i := 0; i < 10; i++ {
		store.add(&types.Block{
			Header: &types.Header{
				Number: uint64(i),
			},
			Transactions: []*types.Transaction{{From: addr0}},
		})
	}

	eth := newTestEthEndpoint(store)

	cases := []struct {
		blockNum BlockNumber
		isNotNil bool
		err      bool
	}{
		{LatestBlockNumber, true, false},
		{EarliestBlockNumber, true, false},
		{BlockNumber(-50), false, true},
		{BlockNumber(0), true, false},
		{BlockNumber(2), true, false},
		{BlockNumber(50), false, false},
	}
	for _, c := range cases {
		res, err := eth.GetBlockTransactionCountByNumber(c.blockNum)

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
	store := &mockBlockStore{}
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

	eth := newTestEthEndpoint(store)

	for _, testCase := range testTable {
		t.Run(testCase.name, func(t *testing.T) {
			foundLogs, logError := eth.GetLogs(
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

type mockBlockStore struct {
	ethStore
	blocks []*types.Block
	topics []types.Hash
}

func (m *mockBlockStore) add(blocks ...*types.Block) {
	if m.blocks == nil {
		m.blocks = []*types.Block{}
	}

	m.blocks = append(m.blocks, blocks...)
}

func (m *mockBlockStore) GetReceiptsByHash(hash types.Hash) ([]*types.Receipt, error) {
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

func (m *mockBlockStore) GetHeaderByNumber(blockNumber uint64) (*types.Header, bool) {
	b, ok := m.GetBlockByNumber(blockNumber, false)
	if !ok {
		return nil, false
	}

	return b.Header, true
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
