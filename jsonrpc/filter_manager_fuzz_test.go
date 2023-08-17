package jsonrpc

import (
	"math/big"
	"strconv"
	"testing"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
)

func FuzzGetLogsForQuery(f *testing.F) {
	// Topics we're searching for
	topic1 := types.StringToHash("4")
	topic2 := types.StringToHash("5")
	topic3 := types.StringToHash("6")

	var topics = [][]types.Hash{{topic1}, {topic2}, {topic3}}

	// setup test
	store := &mockBlockStore{
		topics: []types.Hash{topic1, topic2, topic3},
	}
	store.setupLogs()

	blocks := make([]*types.Block, 5)

	for i := range blocks {
		blocks[i] = &types.Block{
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
		}
	}

	store.appendBlocksToStore(blocks)

	fm := NewFilterManager(hclog.NewNullLogger(), store, 1000)

	f.Cleanup(func() {
		defer fm.Close()
	})

	seeds := []struct {
		fromBlock int64
		toBlock   int64
		blockHash []byte
	}{
		{
			blockHash: types.StringToHash("1").Bytes(),
			fromBlock: 1,
			toBlock:   3,
		},
		{
			blockHash: types.StringToHash("2").Bytes(),
			fromBlock: 2,
			toBlock:   2,
		},
		{},
		{
			blockHash: types.StringToHash("3").Bytes(),
			fromBlock: 10,
			toBlock:   5,
		},
		{
			blockHash: types.StringToHash("4").Bytes(),
			fromBlock: 10,
			toBlock:   1012,
		},
	}

	for _, seed := range seeds {
		f.Add(seed.blockHash, seed.fromBlock, seed.toBlock)
	}

	f.Fuzz(func(t *testing.T, hash []byte, fromBlock int64, toBlock int64) {
		if len(hash) != types.HashLength {
			t.Skip()
		}
		blockHash := types.BytesToHash(hash)

		logQuery := LogQuery{
			BlockHash: &blockHash,
			fromBlock: BlockNumber(fromBlock),
			toBlock:   BlockNumber(toBlock),
			Topics:    topics,
		}
		_, _ = fm.GetLogsForQuery(&logQuery)
	})
}

func FuzzGetLogFilterFromID(f *testing.F) {
	store := newMockStore()

	m := NewFilterManager(hclog.NewNullLogger(), store, 1000)
	defer m.Close()

	go m.Run()

	seeds := []struct {
		address   []byte
		toBlock   int64
		fromBlock int64
	}{
		{
			address:   types.StringToAddress("1").Bytes(),
			toBlock:   10,
			fromBlock: 0,
		},
		{
			address:   types.StringToAddress("2").Bytes(),
			toBlock:   0,
			fromBlock: -4,
		},
		{
			address:   types.StringToAddress("40").Bytes(),
			toBlock:   34,
			fromBlock: 5,
		},
		{
			address:   types.StringToAddress("12").Bytes(),
			toBlock:   -1,
			fromBlock: -5,
		},
	}

	for _, seed := range seeds {
		f.Add(seed.address, seed.toBlock, seed.fromBlock)
	}

	f.Fuzz(func(t *testing.T, address []byte, toBlock int64, fromBlock int64) {
		if len(address) != types.AddressLength {
			t.Skip()
		}
		logFilter := &LogQuery{
			Addresses: []types.Address{types.BytesToAddress(address)},
			toBlock:   BlockNumber(toBlock),
			fromBlock: BlockNumber(fromBlock),
		}
		retrivedLogFilter, err := m.GetLogFilterFromID(
			m.NewLogFilter(logFilter, &MockClosedWSConnection{}),
		)
		if err != nil {
			assert.Equal(t, logFilter, retrivedLogFilter.query)
		}
	})
}
