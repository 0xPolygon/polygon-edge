package jsonrpc

import (
	"errors"
	"fmt"
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
)

func createTestTransaction(hash types.Hash) *types.Transaction {
	return &types.Transaction{
		Hash: hash,
	}
}

func createTestHeader(height uint64) *types.Header {
	h := &types.Header{
		Number: height,
	}

	h.ComputeHash()

	return h
}

func wrapHeaderWithTestBlock(h *types.Header) *types.Block {
	return &types.Block{
		Header: h,
	}
}

var (
	testTxHash1 = types.BytesToHash([]byte{1})
	testTx1     = createTestTransaction(testTxHash1)

	testGenesisHeader = createTestHeader(0)
	testGenesisBlock  = wrapHeaderWithTestBlock(testGenesisHeader)

	testLatestHeader = createTestHeader(100)
	testLatestBlock  = wrapHeaderWithTestBlock(testLatestHeader)

	testHeader10 = createTestHeader(10)
	testBlock10  = wrapHeaderWithTestBlock(testHeader10)

	testHash11 = types.BytesToHash([]byte{11})

	testTraceResult = map[string]interface{}{
		"failed":      false,
		"gas":         1000,
		"returnValue": "test return",
		"structLogs": []interface{}{
			"log1",
			"log2",
		},
	}
	testTraceResults = []interface{}{
		testTraceResult,
	}
)

func TestGetNumericBlockNumber(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		num      BlockNumber
		store    latestHeaderGetter
		expected uint64
		err      error
	}{
		{
			name: "should return the latest block's number if it is found",
			num:  LatestBlockNumber,
			store: &debugEndpointMockStore{
				headerFn: func() *types.Header {
					return &types.Header{
						Number: 10,
					}
				},
			},
			expected: 10,
			err:      nil,
		},
		{
			name: "should return error if the latest block's number is not found",
			num:  LatestBlockNumber,
			store: &debugEndpointMockStore{
				headerFn: func() *types.Header {
					return nil
				},
			},
			expected: 0,
			err:      ErrLatestNotFound,
		},
		{
			name:     "should return 0 number if earliest is given",
			num:      EarliestBlockNumber,
			store:    &debugEndpointMockStore{},
			expected: 0,
			err:      nil,
		},
		{
			name: "should return latest if found and pending is given",
			num:  PendingBlockNumber,
			store: &debugEndpointMockStore{
				headerFn: func() *types.Header {
					return &types.Header{
						Number: 10,
					}
				},
			},
			expected: 10,
			err:      nil,
		},
		{
			name: "should return error if given pending and the latest block's number is not found",
			num:  PendingBlockNumber,
			store: &debugEndpointMockStore{
				headerFn: func() *types.Header {
					return nil
				},
			},
			expected: 0,
			err:      ErrLatestNotFound,
		},
		{
			name: "should return error for latest if not found and pending is given",
			num:  PendingBlockNumber,
			store: &debugEndpointMockStore{
				headerFn: func() *types.Header {
					return &types.Header{
						Number: 10,
					}
				},
			},
			expected: 10,
			err:      nil,
		},
		{
			name:     "should return error if negative number is given",
			num:      -5,
			store:    &debugEndpointMockStore{},
			expected: 0,
			err:      ErrNegativeBlockNumber,
		},
		{
			name:     "should return the given block number otherwise",
			num:      5,
			store:    &debugEndpointMockStore{},
			expected: 5,
			err:      nil,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			res, err := GetNumericBlockNumber(test.num, test.store)

			assert.Equal(t, test.expected, res)
			assert.Equal(t, test.err, err)
		})
	}
}

func TestGetBlockHeader(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		num      BlockNumber
		store    headerGetter
		expected *types.Header
		err      error
	}{
		{
			name: "should return the latest block's number if latest is given",
			num:  LatestBlockNumber,
			store: &debugEndpointMockStore{
				headerFn: func() *types.Header {
					return testLatestHeader
				},
			},
			expected: testLatestHeader,
			err:      nil,
		},
		{
			name: "should return genesis block if Earliest is given",
			num:  EarliestBlockNumber,
			store: &debugEndpointMockStore{
				getHeaderByNumberFn: func(num uint64) (*types.Header, bool) {
					assert.Zero(t, num)

					return testGenesisHeader, true
				},
			},
			expected: testGenesisHeader,
			err:      nil,
		},
		{
			name: "should return error if genesis header not found",
			num:  EarliestBlockNumber,
			store: &debugEndpointMockStore{
				getHeaderByNumberFn: func(num uint64) (*types.Header, bool) {
					assert.Zero(t, num)

					return nil, false
				},
			},
			expected: nil,
			err:      ErrFailedFetchGenesis,
		},
		{
			name: "should return latest if pending is given",
			num:  PendingBlockNumber,
			store: &debugEndpointMockStore{
				headerFn: func() *types.Header {
					return testLatestHeader
				},
			},
			expected: testLatestHeader,
			err:      nil,
		},
		{
			name: "should return header at arbitrary height",
			num:  10,
			store: &debugEndpointMockStore{
				getHeaderByNumberFn: func(num uint64) (*types.Header, bool) {
					assert.Equal(t, uint64(10), num)

					return testHeader10, true
				},
			},
			expected: testHeader10,
			err:      nil,
		},
		{
			name: "should return error if header not found",
			num:  11,
			store: &debugEndpointMockStore{
				getHeaderByNumberFn: func(num uint64) (*types.Header, bool) {
					assert.Equal(t, uint64(11), num)

					return nil, false
				},
			},
			expected: nil,
			err:      fmt.Errorf("error fetching block number %d header", 11),
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			res, err := GetBlockHeader(test.num, test.store)

			assert.Equal(t, test.expected, res)
			assert.Equal(t, test.err, err)
		})
	}
}

func TestGetTxAndBlockByTxHash(t *testing.T) {
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
		store  txLookupAndBlockGetter
		tx     *types.Transaction
		block  *types.Block
	}{
		{
			name:   "should return tx and block",
			txHash: testTx1.Hash,
			store: &debugEndpointMockStore{
				readTxLookupFn: func(hash types.Hash) (types.Hash, bool) {
					assert.Equal(t, testTx1.Hash, hash)

					return blockWithTx.Hash(), true
				},
				getBlockByHashFn: func(hash types.Hash, full bool) (*types.Block, bool) {
					assert.Equal(t, blockWithTx.Hash(), hash)
					assert.True(t, full)

					return blockWithTx, true
				},
			},
			tx:    testTx1,
			block: blockWithTx,
		},
		{
			name:   "should return nil if ReadTxLookup returns nothing",
			txHash: testTx1.Hash,
			store: &debugEndpointMockStore{
				readTxLookupFn: func(hash types.Hash) (types.Hash, bool) {
					assert.Equal(t, testTx1.Hash, hash)

					return types.ZeroHash, false
				},
			},
			tx:    nil,
			block: nil,
		},
		{
			name:   "should return nil if GetBlockByHash returns nothing",
			txHash: testTx1.Hash,
			store: &debugEndpointMockStore{
				readTxLookupFn: func(hash types.Hash) (types.Hash, bool) {
					assert.Equal(t, testTx1.Hash, hash)

					return blockWithTx.Hash(), true
				},
				getBlockByHashFn: func(hash types.Hash, full bool) (*types.Block, bool) {
					assert.Equal(t, blockWithTx.Hash(), hash)
					assert.True(t, full)

					return nil, false
				},
			},
			tx:    nil,
			block: nil,
		},
		{
			name:   "should return nil if the block doesn't include the tx",
			txHash: testTx1.Hash,
			store: &debugEndpointMockStore{
				readTxLookupFn: func(hash types.Hash) (types.Hash, bool) {
					assert.Equal(t, testTx1.Hash, hash)

					return blockWithTx.Hash(), true
				},
				getBlockByHashFn: func(hash types.Hash, full bool) (*types.Block, bool) {
					assert.Equal(t, blockWithTx.Hash(), hash)
					assert.True(t, full)

					return testBlock10, true
				},
			},
			tx:    nil,
			block: nil,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			tx, block := GetTxAndBlockByTxHash(test.txHash, test.store)

			assert.Equal(t, test.tx, tx)
			assert.Equal(t, test.block, block)
		})
	}
}

func TestGetHeaderFromBlockNumberOrHash(t *testing.T) {
	t.Parallel()

	block10Num := BlockNumber(testBlock10.Number())

	tests := []struct {
		name   string
		bnh    BlockNumberOrHash
		store  *debugEndpointMockStore
		header *types.Header
		err    bool
	}{
		{
			name: "should return latest header if neither block number nor hash is given",
			bnh:  BlockNumberOrHash{},
			store: &debugEndpointMockStore{
				headerFn: func() *types.Header {
					return testLatestHeader
				},
				getBlockByHashFn: func(hash types.Hash, full bool) (*types.Block, bool) {
					assert.Equal(t, testLatestHeader.Hash, hash)
					assert.False(t, full)

					return testLatestBlock, true
				},
			},
			header: testLatestHeader,
			err:    false,
		},
		{
			name: "should return header by number if both block number and hash are given",
			bnh: BlockNumberOrHash{
				BlockNumber: &block10Num,
				BlockHash:   &testLatestBlock.Header.Hash,
			},
			store: &debugEndpointMockStore{
				getHeaderByNumberFn: func(num uint64) (*types.Header, bool) {
					assert.Equal(t, testHeader10.Number, num)

					return testHeader10, true
				},
			},
			header: testHeader10,
			err:    false,
		},
		{
			name: "should return header by hash if both hash are given",
			bnh: BlockNumberOrHash{
				BlockHash: &testHeader10.Hash,
			},
			store: &debugEndpointMockStore{
				getBlockByHashFn: func(hash types.Hash, full bool) (*types.Block, bool) {
					assert.Equal(t, testHeader10.Hash, hash)
					assert.False(t, full)

					return testBlock10, true
				},
			},
			header: testHeader10,
			err:    false,
		},
		{
			name: "should return error if header not found when block number is given",
			bnh: BlockNumberOrHash{
				BlockNumber: &block10Num,
			},
			store: &debugEndpointMockStore{
				getHeaderByNumberFn: func(num uint64) (*types.Header, bool) {
					assert.Equal(t, testHeader10.Number, num)

					return nil, false
				},
			},
			header: nil,
			err:    true,
		},
		{
			name: "should return header by hash if both hash are given",
			bnh: BlockNumberOrHash{
				BlockHash: &testHeader10.Hash,
			},
			store: &debugEndpointMockStore{
				getBlockByHashFn: func(hash types.Hash, full bool) (*types.Block, bool) {
					assert.Equal(t, testHeader10.Hash, hash)
					assert.False(t, full)

					return nil, false
				},
			},
			header: nil,
			err:    true,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			header, err := GetHeaderFromBlockNumberOrHash(test.bnh, test.store)

			assert.Equal(t, test.header, header)

			if test.err {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGetNextNonce(t *testing.T) {
	t.Parallel()

	var (
		testAddr  = types.StringToAddress("1")
		stateRoot = types.StringToHash("2")
		blockNum  = uint64(10)
		nonce     = uint64(16)

		testErr = errors.New("test")
	)

	tests := []struct {
		name     string
		address  types.Address
		num      BlockNumber
		store    nonceGetter
		expected uint64
		err      bool
	}{
		{
			name:    "should return the nonce from txpool if pending num is given",
			address: testAddr,
			num:     PendingBlockNumber,
			store: &debugEndpointMockStore{
				getNonceFn: func(a types.Address) uint64 {
					assert.Equal(t, testAddr, a)

					return nonce
				},
			},
			expected: nonce,
			err:      false,
		},
		{
			name:    "should return state nonce if block number is given",
			address: testAddr,
			num:     BlockNumber(blockNum),
			store: &debugEndpointMockStore{
				getHeaderByNumberFn: func(num uint64) (*types.Header, bool) {
					assert.Equal(t, blockNum, num)

					return &types.Header{
						StateRoot: stateRoot,
					}, true
				},
				getAccountFn: func(hash types.Hash, addr types.Address) (*Account, error) {
					assert.Equal(t, stateRoot, hash)
					assert.Equal(t, testAddr, addr)

					return &Account{
						Nonce: nonce,
					}, nil
				},
			},
			expected: nonce,
			err:      false,
		},
		{
			name:    "should return 0 if account state not found",
			address: testAddr,
			num:     BlockNumber(blockNum),
			store: &debugEndpointMockStore{
				getHeaderByNumberFn: func(num uint64) (*types.Header, bool) {
					assert.Equal(t, blockNum, num)

					return &types.Header{
						StateRoot: stateRoot,
					}, true
				},
				getAccountFn: func(hash types.Hash, addr types.Address) (*Account, error) {
					assert.Equal(t, stateRoot, hash)
					assert.Equal(t, testAddr, addr)

					return nil, ErrStateNotFound
				},
			},
			expected: 0,
			err:      false,
		},
		{
			name:    "should return error if block header not found",
			address: testAddr,
			num:     BlockNumber(blockNum),
			store: &debugEndpointMockStore{
				getHeaderByNumberFn: func(num uint64) (*types.Header, bool) {
					assert.Equal(t, blockNum, num)

					return nil, false
				},
			},
			expected: 0,
			err:      true,
		},
		{
			name:    "should return error if getting state fails",
			address: testAddr,
			num:     BlockNumber(blockNum),
			store: &debugEndpointMockStore{
				getHeaderByNumberFn: func(num uint64) (*types.Header, bool) {
					assert.Equal(t, blockNum, num)

					return &types.Header{
						StateRoot: stateRoot,
					}, true
				},
				getAccountFn: func(hash types.Hash, addr types.Address) (*Account, error) {
					assert.Equal(t, stateRoot, hash)
					assert.Equal(t, testAddr, addr)

					return nil, testErr
				},
			},
			expected: 0,
			err:      true,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			nonce, err := GetNextNonce(test.address, test.num, test.store)

			assert.Equal(t, test.expected, nonce)

			if test.err {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDecodeTxn(t *testing.T) {
	t.Parallel()

	var (
		from       = types.StringToAddress("1")
		to         = types.StringToAddress("2")
		gas        = argUint64(uint64(1))
		gasPrice   = argBytes(new(big.Int).SetUint64(2).Bytes())
		value      = argBytes(new(big.Int).SetUint64(4).Bytes())
		data       = argBytes(new(big.Int).SetUint64(8).Bytes())
		input      = argBytes(new(big.Int).SetUint64(16).Bytes())
		nonce      = argUint64(uint64(32))
		stateNonce = argUint64(uint64(64))

		testError = errors.New("test error")
	)

	tests := []struct {
		name     string
		arg      *txnArgs
		store    nonceGetter
		expected *types.Transaction
		err      bool
	}{
		{
			name: "should return mapped transaction",
			arg: &txnArgs{
				From:     &from,
				To:       &to,
				Gas:      &gas,
				GasPrice: &gasPrice,
				Value:    &value,
				Input:    &input,
				Nonce:    &nonce,
			},
			store: &debugEndpointMockStore{},
			expected: &types.Transaction{
				From:     from,
				To:       &to,
				Gas:      uint64(gas),
				GasPrice: new(big.Int).SetBytes([]byte(gasPrice)),
				Value:    new(big.Int).SetBytes([]byte(value)),
				Input:    input,
				Nonce:    uint64(nonce),
			},
			err: false,
		},
		{
			name: "should set zero address to from and 0 to nonce if from is not given",
			arg: &txnArgs{
				To:       &to,
				Gas:      &gas,
				GasPrice: &gasPrice,
				Value:    &value,
				Input:    &input,
				Nonce:    &nonce,
			},
			store: &debugEndpointMockStore{},
			expected: &types.Transaction{
				From:     types.ZeroAddress,
				To:       &to,
				Gas:      uint64(gas),
				GasPrice: new(big.Int).SetBytes([]byte(gasPrice)),
				Value:    new(big.Int).SetBytes([]byte(value)),
				Input:    input,
				Nonce:    uint64(0),
			},
			err: false,
		},
		{
			name: "should get from store if nonce is not given",
			arg: &txnArgs{
				From:     &from,
				To:       &to,
				Gas:      &gas,
				GasPrice: &gasPrice,
				Value:    &value,
				Input:    &input,
			},
			store: &debugEndpointMockStore{
				headerFn: func() *types.Header {
					return testLatestHeader
				},
				getAccountFn: func(hash types.Hash, addr types.Address) (*Account, error) {
					assert.Equal(t, from, addr)

					return &Account{
						Nonce: uint64(stateNonce),
					}, nil
				},
			},
			expected: &types.Transaction{
				From:     from,
				To:       &to,
				Gas:      uint64(gas),
				GasPrice: new(big.Int).SetBytes([]byte(gasPrice)),
				Value:    new(big.Int).SetBytes([]byte(value)),
				Input:    input,
				Nonce:    uint64(stateNonce),
			},
			err: false,
		},
		{
			name: "should give priority to data than input if both are given",
			arg: &txnArgs{
				From:     &from,
				To:       &to,
				Gas:      &gas,
				GasPrice: &gasPrice,
				Value:    &value,
				Data:     &data,
				Input:    &input,
				Nonce:    &nonce,
			},
			store: &debugEndpointMockStore{},
			expected: &types.Transaction{
				From:     from,
				To:       &to,
				Gas:      uint64(gas),
				GasPrice: new(big.Int).SetBytes([]byte(gasPrice)),
				Value:    new(big.Int).SetBytes([]byte(value)),
				Input:    data,
				Nonce:    uint64(nonce),
			},
			err: false,
		},
		{
			name: "should set zero to value, gas price, input, and gas as default",
			arg: &txnArgs{
				From:  &from,
				To:    &to,
				Nonce: &nonce,
			},
			store: &debugEndpointMockStore{},
			expected: &types.Transaction{
				From:     from,
				To:       &to,
				Gas:      uint64(0),
				GasPrice: new(big.Int),
				Value:    new(big.Int),
				Input:    []byte{},
				Nonce:    uint64(nonce),
			},
			err: false,
		},
		{
			name: "should return error if nonce fetch fails",
			arg: &txnArgs{
				From:     &from,
				To:       &to,
				Gas:      &gas,
				GasPrice: &gasPrice,
				Value:    &value,
				Input:    &input,
			},
			store: &debugEndpointMockStore{
				headerFn: func() *types.Header {
					return testLatestHeader
				},
				getAccountFn: func(hash types.Hash, addr types.Address) (*Account, error) {
					assert.Equal(t, from, addr)

					return nil, testError
				},
			},
			expected: nil,
			err:      true,
		},
		{
			name: "should return error both to and input are not given",
			arg: &txnArgs{
				From:     &from,
				Gas:      &gas,
				GasPrice: &gasPrice,
				Value:    &value,
				Nonce:    &nonce,
			},
			store:    &debugEndpointMockStore{},
			expected: nil,
			err:      true,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			tx, err := DecodeTxn(test.arg, test.store)

			// DecodeTxn computes hash of tx
			if !test.err {
				test.expected.ComputeHash()
			}

			assert.Equal(t, test.expected, tx)

			if test.err {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
