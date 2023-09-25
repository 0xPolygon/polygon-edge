package jsonrpc

import (
	"encoding/json"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/state/runtime/tracer"
	"github.com/0xPolygon/polygon-edge/state/runtime/tracer/structtracer"
	"github.com/0xPolygon/polygon-edge/types"
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
		{
			input: `{
				"disableStack": true,
				"disableStorage": true,
				"enableReturnData": true,
				"disableStructLogs": true
			}`,
			expected: TraceConfig{
				DisableStack:      true,
				DisableStorage:    true,
				EnableReturnData:  true,
				DisableStructLogs: true,
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

func TestTraceBlockByNumber(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		blockNumber BlockNumber
		config      *TraceConfig
		store       *debugEndpointMockStore
		result      interface{}
		err         bool
	}{
		{
			name:        "should trace the latest block",
			blockNumber: LatestBlockNumber,
			config:      &TraceConfig{},
			store: &debugEndpointMockStore{
				headerFn: func() *types.Header {
					return testLatestHeader
				},
				getBlockByNumberFn: func(num uint64, full bool) (*types.Block, bool) {
					assert.Equal(t, testLatestHeader.Number, num)
					assert.True(t, full)

					return testLatestBlock, true
				},
				traceBlockFn: func(block *types.Block, tracer tracer.Tracer) ([]interface{}, error) {
					assert.Equal(t, testLatestBlock, block)

					return testTraceResults, nil
				},
			},
			result: testTraceResults,
			err:    false,
		},
		{
			name:        "should trace the block at the given height",
			blockNumber: 10,
			config:      &TraceConfig{},
			store: &debugEndpointMockStore{
				getBlockByNumberFn: func(num uint64, full bool) (*types.Block, bool) {
					assert.Equal(t, testHeader10.Number, num)
					assert.True(t, full)

					return testBlock10, true
				},
				traceBlockFn: func(block *types.Block, tracer tracer.Tracer) ([]interface{}, error) {
					assert.Equal(t, testBlock10, block)

					return testTraceResults, nil
				},
			},
			result: testTraceResults,
			err:    false,
		},
		{
			name:        "should return errTraceGenesisBlock for genesis block",
			blockNumber: 0,
			config:      &TraceConfig{},
			store: &debugEndpointMockStore{
				getBlockByNumberFn: func(num uint64, full bool) (*types.Block, bool) {
					assert.Equal(t, testGenesisHeader.Number, num)
					assert.True(t, full)

					return testGenesisBlock, true
				},
			},
			result: nil,
			err:    true,
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
			err:    true,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			endpoint := NewDebug(test.store, 100000)

			res, err := endpoint.TraceBlockByNumber(test.blockNumber, test.config)

			assert.Equal(t, test.result, res)

			if test.err {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
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
		err       bool
	}{
		{
			name:      "should trace the latest block",
			blockHash: testHeader10.Hash,
			config:    &TraceConfig{},
			store: &debugEndpointMockStore{
				getBlockByHashFn: func(hash types.Hash, full bool) (*types.Block, bool) {
					assert.Equal(t, testHeader10.Hash, hash)
					assert.True(t, full)

					return testBlock10, true
				},
				traceBlockFn: func(block *types.Block, tracer tracer.Tracer) ([]interface{}, error) {
					assert.Equal(t, testBlock10, block)

					return testTraceResults, nil
				},
			},
			result: testTraceResults,
			err:    false,
		},
		{
			name:      "should return errBlockNotFound",
			blockHash: testHash11,
			config:    &TraceConfig{},
			store: &debugEndpointMockStore{
				getBlockByHashFn: func(hash types.Hash, full bool) (*types.Block, bool) {
					assert.Equal(t, testHash11, hash)
					assert.True(t, full)

					return nil, false
				},
			},
			result: nil,
			err:    true,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			endpoint := NewDebug(test.store, 100000)

			res, err := endpoint.TraceBlockByHash(test.blockHash, test.config)

			assert.Equal(t, test.result, res)

			if test.err {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestTraceBlock(t *testing.T) {
	t.Parallel()

	blockBytes := testLatestBlock.MarshalRLP()
	blockHex := hex.EncodeToHex(blockBytes)

	tests := []struct {
		name   string
		input  string
		config *TraceConfig
		store  *debugEndpointMockStore
		result interface{}
		err    bool
	}{
		{
			name:   "should trace the given block",
			input:  blockHex,
			config: &TraceConfig{},
			store: &debugEndpointMockStore{
				traceBlockFn: func(block *types.Block, tracer tracer.Tracer) ([]interface{}, error) {
					assert.Equal(t, testLatestBlock, block)

					return testTraceResults, nil
				},
			},
			result: testTraceResults,
			err:    false,
		},
		{
			name:   "should return error in case of invalid block",
			input:  "hoge",
			config: &TraceConfig{},
			store:  &debugEndpointMockStore{},
			result: nil,
			err:    true,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			endpoint := NewDebug(test.store, 100000)

			res, err := endpoint.TraceBlock(test.input, test.config)

			assert.Equal(t, test.result, res)

			if test.err {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestTraceTransaction(t *testing.T) {
	t.Parallel()

	blockWithTx := &types.Block{
		Header: testBlock10.Header,
		Transactions: []*types.Transaction{
			testTx1,
		},
	}

	tests := []struct {
		name   string
		txHash types.Hash
		config *TraceConfig
		store  *debugEndpointMockStore
		result interface{}
		err    bool
	}{
		{
			name:   "should trace the given transaction",
			txHash: testTxHash1,
			config: &TraceConfig{},
			store: &debugEndpointMockStore{
				readTxLookupFn: func(hash types.Hash) (types.Hash, bool) {
					assert.Equal(t, testTxHash1, hash)

					return testBlock10.Hash(), true
				},
				getBlockByHashFn: func(hash types.Hash, full bool) (*types.Block, bool) {
					assert.Equal(t, testBlock10.Hash(), hash)
					assert.True(t, full)

					return blockWithTx, true
				},
				traceTxnFn: func(block *types.Block, txHash types.Hash, tracer tracer.Tracer) (interface{}, error) {
					assert.Equal(t, blockWithTx, block)
					assert.Equal(t, testTxHash1, txHash)

					return testTraceResult, nil
				},
			},
			result: testTraceResult,
			err:    false,
		},
		{
			name:   "should return error if ReadTxLookup returns null",
			txHash: testTxHash1,
			config: &TraceConfig{},
			store: &debugEndpointMockStore{
				readTxLookupFn: func(hash types.Hash) (types.Hash, bool) {
					assert.Equal(t, testTxHash1, hash)

					return types.ZeroHash, false
				},
			},
			result: nil,
			err:    true,
		},
		{
			name:   "should return error if block not found",
			txHash: testTxHash1,
			config: &TraceConfig{},
			store: &debugEndpointMockStore{
				readTxLookupFn: func(hash types.Hash) (types.Hash, bool) {
					assert.Equal(t, testTxHash1, hash)

					return testBlock10.Hash(), true
				},
				getBlockByHashFn: func(hash types.Hash, full bool) (*types.Block, bool) {
					assert.Equal(t, testBlock10.Hash(), hash)
					assert.True(t, full)

					return nil, false
				},
			},
			result: nil,
			err:    true,
		},
		{
			name:   "should return error if the tx is not including the block",
			txHash: testTxHash1,
			config: &TraceConfig{},
			store: &debugEndpointMockStore{
				readTxLookupFn: func(hash types.Hash) (types.Hash, bool) {
					assert.Equal(t, testTxHash1, hash)

					return testBlock10.Hash(), true
				},
				getBlockByHashFn: func(hash types.Hash, full bool) (*types.Block, bool) {
					assert.Equal(t, testBlock10.Hash(), hash)
					assert.True(t, full)

					return testBlock10, true
				},
			},
			result: nil,
			err:    true,
		},
		{
			name:   "should return error if the block is genesis",
			txHash: testTxHash1,
			config: &TraceConfig{},
			store: &debugEndpointMockStore{
				readTxLookupFn: func(hash types.Hash) (types.Hash, bool) {
					assert.Equal(t, testTxHash1, hash)

					return testBlock10.Hash(), true
				},
				getBlockByHashFn: func(hash types.Hash, full bool) (*types.Block, bool) {
					assert.Equal(t, testBlock10.Hash(), hash)
					assert.True(t, full)

					return &types.Block{
						Header: testGenesisHeader,
						Transactions: []*types.Transaction{
							testTx1,
						},
					}, true
				},
			},
			result: nil,
			err:    true,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			endpoint := NewDebug(test.store, 100000)

			res, err := endpoint.TraceTransaction(test.txHash, test.config)

			assert.Equal(t, test.result, res)

			if test.err {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestTraceCall(t *testing.T) {
	t.Parallel()

	var (
		from      = types.StringToAddress("1")
		to        = types.StringToAddress("2")
		gas       = argUint64(10000)
		gasPrice  = argBytes(new(big.Int).SetUint64(10).Bytes())
		gasTipCap = argBytes(new(big.Int).SetUint64(10).Bytes())
		gasFeeCap = argBytes(new(big.Int).SetUint64(10).Bytes())
		value     = argBytes(new(big.Int).SetUint64(1000).Bytes())
		data      = argBytes([]byte("data"))
		input     = argBytes([]byte("input"))
		nonce     = argUint64(1)

		blockNumber = BlockNumber(testBlock10.Number())

		txArg = &txnArgs{
			From:      &from,
			To:        &to,
			Gas:       &gas,
			GasPrice:  &gasPrice,
			GasTipCap: &gasTipCap,
			GasFeeCap: &gasFeeCap,
			Value:     &value,
			Data:      &data,
			Input:     &input,
			Nonce:     &nonce,
		}
		decodedTx = &types.Transaction{
			Nonce:     uint64(nonce),
			GasPrice:  new(big.Int).SetBytes([]byte(gasPrice)),
			GasTipCap: new(big.Int).SetBytes([]byte(gasTipCap)),
			GasFeeCap: new(big.Int).SetBytes([]byte(gasFeeCap)),
			Gas:       uint64(gas),
			To:        &to,
			Value:     new(big.Int).SetBytes([]byte(value)),
			Input:     data,
			From:      from,
		}
	)

	decodedTx.ComputeHash(1)

	tests := []struct {
		name   string
		arg    *txnArgs
		filter BlockNumberOrHash
		config *TraceConfig
		store  *debugEndpointMockStore
		result interface{}
		err    bool
	}{
		{
			name: "should trace the given transaction",
			arg:  txArg,
			filter: BlockNumberOrHash{
				BlockNumber: &blockNumber,
			},
			config: &TraceConfig{},
			store: &debugEndpointMockStore{
				getHeaderByNumberFn: func(num uint64) (*types.Header, bool) {
					assert.Equal(t, testBlock10.Number(), num)

					return testHeader10, true
				},
				traceCallFn: func(tx *types.Transaction, header *types.Header, tracer tracer.Tracer) (interface{}, error) {
					assert.Equal(t, decodedTx, tx)
					assert.Equal(t, testHeader10, header)

					return testTraceResult, nil
				},
				headerFn: func() *types.Header {
					return testLatestHeader
				},
				getAccountFn: func(h types.Hash, a types.Address) (*Account, error) {
					return &Account{Nonce: 1}, nil
				},
			},
			result: testTraceResult,
			err:    false,
		},
		{
			name: "should return error if block not found",
			arg:  txArg,
			filter: BlockNumberOrHash{
				BlockHash: &testHeader10.Hash,
			},
			config: &TraceConfig{},
			store: &debugEndpointMockStore{
				getBlockByHashFn: func(hash types.Hash, full bool) (*types.Block, bool) {
					assert.Equal(t, testHeader10.Hash, hash)
					assert.False(t, full)

					return nil, false
				},
				headerFn: func() *types.Header {
					return testLatestHeader
				},
				getAccountFn: func(h types.Hash, a types.Address) (*Account, error) {
					return &Account{Nonce: 1}, nil
				},
			},
			result: nil,
			err:    true,
		},
		{
			name: "should return error if decoding transaction fails",
			arg: &txnArgs{
				From:     &from,
				Gas:      &gas,
				GasPrice: &gasPrice,
				Value:    &value,
				Nonce:    &nonce,
			},
			filter: BlockNumberOrHash{},
			config: &TraceConfig{},
			store: &debugEndpointMockStore{
				headerFn: func() *types.Header {
					return testLatestHeader
				},
				getAccountFn: func(h types.Hash, a types.Address) (*Account, error) {
					return &Account{Nonce: 1}, nil
				},
			},
			result: nil,
			err:    true,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			endpoint := NewDebug(test.store, 100000)

			res, err := endpoint.TraceCall(test.arg, test.filter, test.config)

			assert.Equal(t, test.result, res)

			if test.err {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_newTracer(t *testing.T) {
	t.Parallel()

	t.Run("should create tracer", func(t *testing.T) {
		t.Parallel()

		tracer, cancel, err := newTracer(&TraceConfig{
			EnableMemory:     true,
			EnableReturnData: true,
			DisableStack:     false,
			DisableStorage:   false,
		})

		t.Cleanup(func() {
			cancel()
		})

		assert.NotNil(t, tracer)
		assert.NoError(t, err)
	})

	t.Run("should return error if arg is nil", func(t *testing.T) {
		t.Parallel()

		tracer, cancel, err := newTracer(nil)

		assert.Nil(t, tracer)
		assert.Nil(t, cancel)
		assert.ErrorIs(t, ErrNoConfig, err)
	})

	t.Run("GetResult should return errExecutionTimeout if timeout happens", func(t *testing.T) {
		t.Parallel()

		timeout := "0s"
		tracer, cancel, err := newTracer(&TraceConfig{
			EnableMemory:     true,
			EnableReturnData: true,
			DisableStack:     false,
			DisableStorage:   false,
			Timeout:          &timeout,
		})

		t.Cleanup(func() {
			cancel()
		})

		assert.NoError(t, err)

		// wait until timeout
		time.Sleep(100 * time.Millisecond)

		res, err := tracer.GetResult()
		assert.Nil(t, res)
		assert.Equal(t, ErrExecutionTimeout, err)
	})

	t.Run("GetResult should not return if cancel is called beforre timeout", func(t *testing.T) {
		t.Parallel()

		timeout := "5s"
		tracer, cancel, err := newTracer(&TraceConfig{
			EnableMemory:     true,
			EnableReturnData: true,
			DisableStack:     false,
			DisableStorage:   false,
			Timeout:          &timeout,
		})

		assert.NoError(t, err)

		cancel()

		res, err := tracer.GetResult()

		assert.NotNil(t, res)
		assert.NoError(t, err)
	})

	t.Run("should disable everything if struct logs are disabled", func(t *testing.T) {
		t.Parallel()

		tracer, cancel, err := newTracer(&TraceConfig{
			EnableMemory:      true,
			EnableReturnData:  true,
			DisableStack:      false,
			DisableStorage:    false,
			DisableStructLogs: true,
		})

		t.Cleanup(func() {
			cancel()
		})

		assert.NoError(t, err)

		st, ok := tracer.(*structtracer.StructTracer)
		require.True(t, ok)

		assert.Equal(t, structtracer.Config{
			EnableMemory:     false,
			EnableStack:      false,
			EnableStorage:    false,
			EnableReturnData: true,
			EnableStructLogs: false,
		}, st.Config)
	})
}
