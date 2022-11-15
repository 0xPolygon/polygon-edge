package jsonrpc

import (
	"encoding/json"
	"testing"

	"github.com/0xPolygon/polygon-edge/state/runtime/tracer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
)

type debugEndpointMockStore struct {
	headerFn            func() *types.Header
	getHeaderByNumberFn func(uint64) (*types.Header, bool)
	readTxLookupFn      func(types.Hash) (types.Hash, bool)
	getBlockByHashFn    func(types.Hash, bool) (*types.Block, bool)
	getBlockByNumberFn  func(uint64, bool) (*types.Block, bool)
	traceBlockFn        func(*types.Block, tracer.Tracer) ([]interface{}, error)
	traceTxnFn          func(*types.Block, types.Hash, tracer.Tracer) (interface{}, error)
	traceCallFn         func(*types.Transaction, *types.Header, tracer.Tracer) (interface{}, error)
	getNonceFn          func(types.Address) uint64
	getAccountFn        func(types.Hash, types.Address) (*Account, error)
}

func (s *debugEndpointMockStore) Header() *types.Header {
	return s.headerFn()
}

func (s *debugEndpointMockStore) GetHeaderByNumber(num uint64) (*types.Header, bool) {
	return s.getHeaderByNumberFn(num)
}

func (s *debugEndpointMockStore) ReadTxLookup(txnHash types.Hash) (types.Hash, bool) {
	return s.readTxLookupFn(txnHash)
}

func (s *debugEndpointMockStore) GetBlockByHash(hash types.Hash, full bool) (*types.Block, bool) {
	return s.getBlockByHashFn(hash, full)
}

func (s *debugEndpointMockStore) GetBlockByNumber(num uint64, full bool) (*types.Block, bool) {
	return s.getBlockByNumberFn(num, full)
}

func (s *debugEndpointMockStore) TraceBlock(block *types.Block, tracer tracer.Tracer) ([]interface{}, error) {
	return s.traceBlockFn(block, tracer)
}

func (s *debugEndpointMockStore) TraceTxn(block *types.Block, targetTx types.Hash, tracer tracer.Tracer) (interface{}, error) {
	return s.traceTxnFn(block, targetTx, tracer)
}

func (s *debugEndpointMockStore) TraceCall(tx *types.Transaction, parent *types.Header, tracer tracer.Tracer) (interface{}, error) {
	return s.traceCallFn(tx, parent, tracer)
}

func (s *debugEndpointMockStore) GetNonce(acc types.Address) uint64 {
	return s.getNonceFn(acc)
}

func (s *debugEndpointMockStore) GetAccount(root types.Hash, addr types.Address) (*Account, error) {
	return s.getAccountFn(root, addr)
}

func TestDebugTraceConfigDecode(t *testing.T) {
	timeout15s := "15s"

	tests := []struct {
		input    string
		expected TraceConfig
	}{
		{
			// default
			input: `{}`,
			expected: TraceConfig{
				EnableMemory:     false,
				DisableStack:     false,
				DisableStorage:   false,
				EnableReturnData: false,
			},
		},
		{
			input: `{
				"enableMemory": true
			}`,
			expected: TraceConfig{
				EnableMemory:     true,
				DisableStack:     false,
				DisableStorage:   false,
				EnableReturnData: false,
			},
		},
		{
			input: `{
				"disableStack": true
			}`,
			expected: TraceConfig{
				EnableMemory:     false,
				DisableStack:     true,
				DisableStorage:   false,
				EnableReturnData: false,
			},
		},
		{
			input: `{
				"disableStorage": true
			}`,
			expected: TraceConfig{
				EnableMemory:     false,
				DisableStack:     false,
				DisableStorage:   true,
				EnableReturnData: false,
			},
		},
		{
			input: `{
				"enableReturnData": true
			}`,
			expected: TraceConfig{
				EnableMemory:     false,
				DisableStack:     false,
				DisableStorage:   false,
				EnableReturnData: true,
			},
		},
		{
			input: `{
				"timeout": "15s"
			}`,
			expected: TraceConfig{
				EnableMemory:     false,
				DisableStack:     false,
				DisableStorage:   false,
				EnableReturnData: false,
				Timeout:          &timeout15s,
			},
		},
		{
			input: `{
				"enableMemory": true,
				"disableStack": true,
				"disableStorage": true,
				"enableReturnData": true,
				"timeout": "15s"
			}`,
			expected: TraceConfig{
				EnableMemory:     true,
				DisableStack:     true,
				DisableStorage:   true,
				EnableReturnData: true,
				Timeout:          &timeout15s,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			result := TraceConfig{}

			assert.NoError(
				t,
				json.Unmarshal(
					[]byte(test.input),
					&result,
				),
			)

			assert.Equal(
				t,
				test.expected,
				result,
			)
		})
	}
}

func testCreateTestHeader(height uint64, hash types.Hash) *types.Header {
	return &types.Header{
		Number: height,
		Hash:   hash,
	}
}

func testWrapHeaderWithEmptyBlock(h *types.Header) *types.Block {
	return &types.Block{
		Header:       h,
		Transactions: []*types.Transaction{},
	}
}

var (
	// test data
	genesisHeader = testCreateTestHeader(0, types.BytesToHash([]byte{0}))
	genesisBlock  = testWrapHeaderWithEmptyBlock(genesisHeader)

	latestHeader = testCreateTestHeader(100, types.BytesToHash([]byte{100}))
	latestBlock  = testWrapHeaderWithEmptyBlock(latestHeader)

	hash10   = types.BytesToHash([]byte{10})
	header10 = testCreateTestHeader(10, hash10)
	block10  = testWrapHeaderWithEmptyBlock(header10)

	hash11 = types.BytesToHash([]byte{11})

	sampleResult = map[string]interface{}{
		"failed":      false,
		"gas":         1000,
		"returnValue": "test return",
		"structLogs": []interface{}{
			"log1",
			"log2",
		},
	}
)

func TestTraceBlockByNumber(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		blockNumber BlockNumber
		config      *TraceConfig
		store       *debugEndpointMockStore
		result      interface{}
		err         error
	}{
		{
			name:        "should trace the latest block",
			blockNumber: LatestBlockNumber,
			config:      &TraceConfig{},
			store: &debugEndpointMockStore{
				headerFn: func() *types.Header {
					return latestHeader
				},
				getBlockByNumberFn: func(num uint64, full bool) (*types.Block, bool) {
					assert.Equal(t, latestHeader.Number, num)
					assert.True(t, full)

					return latestBlock, true
				},
				traceBlockFn: func(block *types.Block, tracer tracer.Tracer) ([]interface{}, error) {
					assert.Equal(t, latestBlock, block)

					return []interface{}{sampleResult}, nil
				},
			},
			result: []interface{}{sampleResult},
			err:    nil,
		},
		{
			name:        "should trace the block at the given height",
			blockNumber: 10,
			config:      &TraceConfig{},
			store: &debugEndpointMockStore{
				getBlockByNumberFn: func(num uint64, full bool) (*types.Block, bool) {
					assert.Equal(t, header10.Number, num)
					assert.True(t, full)

					return block10, true
				},
				traceBlockFn: func(block *types.Block, tracer tracer.Tracer) ([]interface{}, error) {
					assert.Equal(t, block10, block)

					return []interface{}{sampleResult}, nil
				},
			},
			result: []interface{}{sampleResult},
			err:    nil,
		},
		{
			name:        "should return errTraceGenesisBlock for genesis block",
			blockNumber: 0,
			config:      &TraceConfig{},
			store: &debugEndpointMockStore{
				getBlockByNumberFn: func(num uint64, full bool) (*types.Block, bool) {
					assert.Equal(t, genesisHeader.Number, num)
					assert.True(t, full)

					return genesisBlock, true
				},
			},
			result: nil,
			err:    errTraceGenesisBlock,
		},
		{
			name:        "should return errBlockNotFound",
			blockNumber: 11,
			config:      &TraceConfig{},
			store: &debugEndpointMockStore{
				getBlockByNumberFn: func(num uint64, full bool) (*types.Block, bool) {
					assert.Equal(t, uint64(11), num)
					assert.True(t, full)

					return nil, false
				},
			},
			result: nil,
			err:    errBlockNotFound,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			endpoint := &Debug{test.store}

			res, err := endpoint.TraceBlockByNumber(test.blockNumber, test.config)

			assert.Equal(t, test.result, res)
			assert.Equal(t, test.err, err)
		})
	}
}

func TestTraceBlockByHash(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		blockHash types.Hash
		config    *TraceConfig
		store     *debugEndpointMockStore
		result    interface{}
		err       error
	}{
		{
			name:      "should trace the latest block",
			blockHash: hash10,
			config:    &TraceConfig{},
			store: &debugEndpointMockStore{
				getBlockByHashFn: func(hash types.Hash, full bool) (*types.Block, bool) {
					assert.Equal(t, hash10, hash)
					assert.True(t, full)

					return block10, true
				},
				traceBlockFn: func(block *types.Block, tracer tracer.Tracer) ([]interface{}, error) {
					assert.Equal(t, block10, block)

					return []interface{}{sampleResult}, nil
				},
			},
			result: []interface{}{sampleResult},
			err:    nil,
		},
		{
			name:      "should return errBlockNotFound",
			blockHash: hash11,
			config:    &TraceConfig{},
			store: &debugEndpointMockStore{
				getBlockByHashFn: func(hash types.Hash, full bool) (*types.Block, bool) {
					assert.Equal(t, hash11, hash)
					assert.True(t, full)

					return nil, false
				},
			},
			result: nil,
			err:    errBlockNotFound,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			endpoint := &Debug{test.store}

			res, err := endpoint.TraceBlockByHash(test.blockHash, test.config)

			assert.Equal(t, test.result, res)
			assert.Equal(t, test.err, err)
		})
	}
}
