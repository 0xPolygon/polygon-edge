package txpool

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-sdk/chain"
	"github.com/0xPolygon/polygon-sdk/crypto"
	"github.com/0xPolygon/polygon-sdk/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
)

const (
	defaultPriceLimit uint64 = 1
	defaultMaxSlots   uint64 = 4096
	validGasLimit     uint64 = 100000
)

var (
	forks = &chain.Forks{
		Homestead: chain.NewFork(0),
		Istanbul:  chain.NewFork(0),
	}

	nilMetrics = NilMetrics()
)

/* MOCK */

type defaultMockStore struct {
}

func (m defaultMockStore) GetNonce(types.Hash, types.Address) uint64 {
	return 0
}

func (m defaultMockStore) GetBlockByHash(types.Hash, bool) (*types.Block, bool) {
	return nil, false
}

func (m defaultMockStore) GetBalance(types.Hash, types.Address) (*big.Int, error) {
	balance, _ := big.NewInt(0).SetString("10000000000000000000", 10)
	return balance, nil
}

func (m defaultMockStore) Header() *types.Header {
	return &types.Header{}
}

type faultyMockStore struct {
}

func (fms faultyMockStore) Header() *types.Header {
	return &types.Header{}
}

func (fms faultyMockStore) GetNonce(root types.Hash, addr types.Address) uint64 {
	return 0
}

func (fms faultyMockStore) GetBlockByHash(hash types.Hash, b bool) (*types.Block, bool) {
	return nil, false
}

func (fms faultyMockStore) GetBalance(root types.Hash, addr types.Address) (*big.Int, error) {
	return nil, fmt.Errorf("unable to fetch account state")
}

type mockSigner struct {
}

func (s *mockSigner) Sender(tx *types.Transaction) (types.Address, error) {
	return tx.From, nil
}

// Mock json-RPC/gRPC and gossip origins
func (p *TxPool) addTestTx(tx *types.Transaction) error {
	return p.addTx(tx)
}

// expected pool state for some account after test run
type result struct {
	enqueued  int    // number of enqueued txs
	promoted  int    // number of promoted txs
	nextNonce uint64 // next nonce (found in nonceMap)
}

func TestAddTxErrors(t *testing.T) {
	testCases := []struct {
		name          string
		expectedError error
		tx            *types.Transaction
	}{
		{
			name:          "ErrNegativeValue",
			expectedError: ErrNegativeValue,
			tx: &types.Transaction{
				From:     types.Address{0x1},
				Value:    big.NewInt(-5),
				GasPrice: big.NewInt(1),
				Gas:      validGasLimit,
			},
		},
		{
			name:          "ErrNonEncryptedTx",
			expectedError: ErrNonEncryptedTx,
			tx: &types.Transaction{
				From:     types.Address{0x1},
				Value:    big.NewInt(0),
				GasPrice: big.NewInt(1),
				Gas:      validGasLimit,
			},
		},
		{
			name:          "ErrInvalidSender",
			expectedError: ErrInvalidSender,
			tx: &types.Transaction{
				From:     types.ZeroAddress,
				Value:    big.NewInt(0),
				GasPrice: big.NewInt(1),
				Gas:      validGasLimit,
			},
		},
		{
			name:          "ErrInvalidAccountState",
			expectedError: ErrInvalidAccountState,
			tx: &types.Transaction{
				From:     types.Address{0x1},
				Value:    big.NewInt(0),
				GasPrice: big.NewInt(1),
				Gas:      validGasLimit,
			},
		},
		// TODO
		// {
		// 	name:          "ErrAlreadyKnown",
		// 	expectedError: ErrAlreadyKnown,
		// 	tx: &types.Transaction{
		// 		From:     types.Address{0x1},
		// 		Value:    big.NewInt(0),
		// 		GasPrice: big.NewInt(1),
		// 		Gas:      validGasLimit,
		// 	},
		// },
		{
			name:          "ErrTxPoolOverflow",
			expectedError: ErrTxPoolOverflow,
			tx: &types.Transaction{
				From:     types.Address{0x1},
				Value:    big.NewInt(0),
				GasPrice: big.NewInt(1),
				Gas:      validGasLimit,
			},
		},
		{
			name:          "ErrIntrinsicGas",
			expectedError: ErrIntrinsicGas,
			tx: &types.Transaction{
				From:     types.Address{0x1},
				Value:    big.NewInt(0),
				GasPrice: big.NewInt(1),
				Gas:      999999,
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			pool, err := NewTxPool(
				hclog.NewNullLogger(),
				forks.At(0),
				defaultMockStore{},
				nil,
				nil,
				nilMetrics,
				&Config{
					MaxSlots: defaultMaxSlots,
					Sealing:  false,
				},
			)
			assert.NoError(t, err)
			pool.AddSigner(crypto.NewEIP155Signer(100))

			/* special error setups */
			switch testCase.expectedError {
			case ErrTxPoolOverflow:
				pool.gauge.increase(defaultMaxSlots)
			case ErrAlreadyKnown:
				// TODO
			case ErrInvalidSender:
				pool.EnableDev()
			case ErrInvalidAccountState:
				pool.store = faultyMockStore{}
			}

			assert.ErrorIs(t, pool.addTestTx(testCase.tx), testCase.expectedError)
		})
	}
}

func TestSingleAddRequest(t *testing.T) {
	testCases := []struct {
		name      string
		txNonce   uint64
		nextNonce uint64
		expected  result
	}{
		{
			name:      "signal promotion",
			txNonce:   1,
			nextNonce: 1,
			expected: result{
				nextNonce: 2,
				enqueued:  0,
				promoted:  1,
			},
		},
		{
			name:      "enqueue higher nonce",
			txNonce:   5,
			nextNonce: 2,
			expected: result{
				nextNonce: 2,
				enqueued:  1,
				promoted:  0,
			},
		},
		{
			name:      "reject low nonce",
			txNonce:   3,
			nextNonce: 6,
			expected: result{
				nextNonce: 3,
				enqueued:  0,
				promoted:  0,
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			// 1. create pool
			pool, err := NewTxPool(
				hclog.NewNullLogger(),
				forks.At(0),
				defaultMockStore{},
				nil,
				nil,
				nilMetrics,
				&Config{
					MaxSlots: defaultMaxSlots,
					Sealing:  false,
				},
			)
			assert.NoError(t, err)
			pool.EnableDev()
			pool.AddSigner(&mockSigner{})
			from := types.Address{0x1}

			// 2. set nonce map to expect
			pool.nonces.store(from, testCase.nextNonce)

			// 3. add tx
			assert.NoError(t,
				pool.addTestTx(
					&types.Transaction{
						From:     from,
						Nonce:    testCase.txNonce,
						Gas:      validGasLimit,
						GasPrice: big.NewInt(1),
						Value:    big.NewInt(0),
					}))

			// 4. Assert
			// enqueued
			assert.Len(t,
				pool.enqueuedMap.accounts[from].length(),
				testCase.expected.enqueued,
			)

			// promoted
			assert.Len(t,
				pool.promoted.length(),
				testCase.expected.enqueued,
			)

			// nonce map
			assert.Equal(t,
				pool.nonces.load(from),
				testCase.expected.nextNonce,
			)
		})
	}
}

func TestSingleAccountMultipleTransactions(t *testing.T) {
	testCases := []struct {
		name     string
		txs      []uint64
		promoted int
		enqueued int
	}{
		{
			name:     "add 7 transactions, promote 3",
			txs:      []uint64{0, 1, 2, 3, 9, 8, 10},
			promoted: 4,
			enqueued: 3,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			// 1. create pool
			pool, err := NewTxPool(
				hclog.NewNullLogger(),
				forks.At(0),
				defaultMockStore{},
				nil,
				nil,
				nilMetrics,
				&Config{
					MaxSlots: defaultMaxSlots,
					Sealing:  false,
				},
			)
			assert.NoError(t, err)
			pool.EnableDev()
			pool.AddSigner(&mockSigner{})
			addr := types.Address{0x1}

			// 2. add transactions
			for _, nonce := range testCase.txs {
				assert.NoError(t, pool.addTestTx(&types.Transaction{
					From:     addr,
					Nonce:    nonce,
					Gas:      validGasLimit,
					GasPrice: big.NewInt(1),
					Value:    big.NewInt(0),
				}))
			}

			// 3. assert enqueued
			assert.Len(t,
				pool.enqueuedMap.accounts[addr].length(),
				testCase.enqueued,
			)

			// 4. assert promoted
			assert.Len(t,
				pool.promoted.length(),
				testCase.promoted,
			)
		})
	}
}

func TestMultipleTransactionsMultipleAccounts(t *testing.T) {

	type accountRequests struct {
		from         types.Address
		txs          []uint64
		currentNonce uint64 // nonce of the next expected tx (nonceMap)
	}

	testCases := []struct {
		name     string
		requests []accountRequests
		expected []result
	}{
		{
			name: "send multiple transactions from 3 accounts",
			requests: []accountRequests{
				{
					from:         types.Address{0x1},
					txs:          []uint64{2, 3, 4, 6, 7, 10},
					currentNonce: 2,
				},
				{
					from:         types.Address{0x2},
					txs:          []uint64{0, 1, 4, 5, 7, 3, 2},
					currentNonce: 2,
				},
				{
					from:         types.Address{0x3},
					txs:          []uint64{4, 3, 1, 6, 7, 31, 3, 1995, 9, 0},
					currentNonce: 1,
				},
			},
			expected: []result{
				{
					nextNonce: 2,
					promoted:  1,
					enqueued:  6,
				},
				{
					nextNonce: 6,
					promoted:  4,
					enqueued:  1,
				},
				{
					nextNonce: 5,
					promoted:  3,
					enqueued:  4,
				},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			// 1. create pool
			pool, err := NewTxPool(
				hclog.NewNullLogger(),
				forks.At(0),
				defaultMockStore{},
				nil,
				nil,
				nilMetrics,
				&Config{
					MaxSlots: defaultMaxSlots,
					Sealing:  false,
				},
			)
			assert.NoError(t, err)
			pool.EnableDev()
			pool.AddSigner(&mockSigner{})

			// 2. send all txs from each account
			for _, req := range testCase.requests {
				// set the current nonce (so promotions can happen)
				pool.nonces.store(req.from, req.currentNonce)

				// send all transaction from address
				for _, nonce := range req.txs {
					assert.NoError(t,
						pool.addTestTx(&types.Transaction{
							From:     req.from,
							Nonce:    nonce,
							Gas:      validGasLimit,
							GasPrice: big.NewInt(1),
							Value:    big.NewInt(0),
						}))
				}
			}

			// 3. Assert
			for i, expected := range testCase.expected {
				// enqueued
				addr := testCase.requests[i].from
				assert.Equal(t,
					pool.enqueuedMap.accounts[addr].length(),
					expected.enqueued,
				)

				// promoted
				assert.Equal(t,
					pool.promoted.length(),
					expected.promoted,
				)

				// nonce map
				assert.Equal(t,
					pool.nonces.load(addr),
					expected.nextNonce,
				)
			}
		})
	}
}

func TestResetRequest(t *testing.T) {
	testCases := []struct {
		name       string
		enqueued   []uint64
		promoted   []uint64
		resetNonce uint64
		expected   result
	}{
		{
			name:       "prune all promoted",
			promoted:   []uint64{3, 4, 5},
			resetNonce: 8,
			expected: result{
				promoted: 0,
			},
		},
		{
			name:       "prune some promoted",
			promoted:   []uint64{0, 1, 2, 3, 4},
			resetNonce: 2,
			expected: result{
				promoted: 2,
			},
		},
		{
			name:       "prune all enqueued",
			enqueued:   []uint64{2, 5, 8},
			resetNonce: 10,
			expected: result{
				enqueued: 0,
			},
		},
		{
			name:       "prune some enqueued",
			enqueued:   []uint64{2, 3, 5, 8, 9},
			resetNonce: 4,
			expected: result{
				enqueued: 3,
			},
		},
		{
			name:       "prune all promoted and some enqueued",
			promoted:   []uint64{3, 4, 5, 6},
			enqueued:   []uint64{8, 9, 10},
			resetNonce: 8,
			expected: result{
				enqueued: 2,
				promoted: 0,
			},
		},
		{
			name:       "prune all promoted and all enqueued",
			promoted:   []uint64{0, 1, 2, 3},
			enqueued:   []uint64{6, 7},
			resetNonce: 11,
			expected: result{
				enqueued: 0,
				promoted: 0,
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			// 1. craete pool
			pool, err := NewTxPool(
				hclog.NewNullLogger(),
				forks.At(0),
				defaultMockStore{},
				nil,
				nil,
				nilMetrics,
				&Config{
					MaxSlots: defaultMaxSlots,
					Sealing:  false,
				},
			)
			assert.NoError(t, err)
			pool.EnableDev()
			pool.AddSigner(&mockSigner{})
			addr := types.Address{0x1}

			// 2. set up pool state before reset event
			var txs []uint64
			txs = append(txs, testCase.promoted...)
			txs = append(txs, testCase.enqueued...)
			// send all txs
			pool.nonces.store(addr, testCase.promoted[0])
			for _, nonce := range txs {
				assert.NoError(t,
					pool.addTestTx(&types.Transaction{
						From:     addr,
						Nonce:    nonce,
						Gas:      validGasLimit,
						GasPrice: big.NewInt(1),
						Value:    big.NewInt(0),
					}))
			}

			// 3. create and send the request (event)
			nonceMap := make(map[types.Address]uint64)
			nonceMap[addr] = testCase.resetNonce
			req := resetRequest{
				newNonces: nonceMap,
			}
			pool.resetReqCh <- req

			// 4. Assert
			// enqueued
			assert.Equal(t,
				pool.enqueuedMap.accounts[addr].length(),
				testCase.expected.enqueued,
			)

			// promoted
			assert.Equal(t,
				pool.promoted.length(),
				testCase.expected.promoted,
			)
		})
	}
}

// helper object to simulate error events
// the actual callback would return
type status int

const (
	recoverable status = iota
	unrecoverable
	abort
	ok
)

// mock Execute callback
type exeTx struct {
	raw    *types.Transaction
	status status
}

func mockExecute(tx exeTx) (bool, bool, bool) {
	switch tx.status {
	case recoverable:
		return false, false, true
	case unrecoverable:
		return false, false, false
	case abort:
		return false, true, false
	default: // ok
		return true, false, false
	}
}

func TestExecuteTransactionsSingleAccount(t *testing.T) {
	type tx struct {
		nonce  uint64
		status status
	}

	testCases := []struct {
		name     string
		promoted []tx
		expected result
	}{
		{
			name: "write 4 recover 3",
			promoted: []tx{
				{nonce: 2, status: ok},
				{nonce: 6, status: recoverable},
				{nonce: 3, status: ok},
				{nonce: 5, status: recoverable},
				{nonce: 8, status: ok},
				{nonce: 4, status: recoverable},
				{nonce: 7, status: ok},
			},
			expected: result{
				promoted: 3,
			},
		},
		{
			name: "rollback all transactions",
			promoted: []tx{
				{nonce: 4, status: unrecoverable},
				{nonce: 5},
				{nonce: 6},
				{nonce: 7},
				{nonce: 8},
			},
			expected: result{
				enqueued: 5,
				promoted: 0,
			},
		},
		{
			name: "rollback some transactions",
			promoted: []tx{
				{nonce: 0, status: ok},
				{nonce: 1, status: recoverable},
				{nonce: 2, status: unrecoverable},
				{nonce: 3},
				{nonce: 4},
			},
			expected: result{
				enqueued: 3,
				promoted: 1,
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			// 1. create pool
			pool, err := NewTxPool(
				hclog.NewNullLogger(),
				forks.At(0),
				defaultMockStore{},
				nil,
				nil,
				nilMetrics,
				&Config{
					MaxSlots: defaultMaxSlots,
					Sealing:  false,
				},
			)
			assert.NoError(t, err)
			pool.EnableDev()
			pool.AddSigner(&mockSigner{})

			// 2. set up pre state
			addr := types.Address{0x1}
			pool.nonces.store(addr, testCase.promoted[0].nonce)
			// save status for mockExecute
			statusMap := make(map[uint64]status)

			// 2. send transactions
			for _, tx := range testCase.promoted {
				statusMap[tx.nonce] = tx.status
				assert.NoError(t,
					pool.addTestTx(&types.Transaction{
						From:     addr,
						Nonce:    tx.nonce,
						Gas:      validGasLimit,
						GasPrice: big.NewInt(1),
						Value:    big.NewInt(0),
					}))
			}

			// 3. Call ExecuteTransactions (block is built)
			pool.ExecuteTransactions(func(tx *types.Transaction) (bool, bool, bool) {
				return mockExecute(exeTx{
					raw:    tx,
					status: statusMap[tx.Nonce],
				})
			})

			// 4. Assert
			// enqueued
			assert.Equal(t,
				pool.enqueuedMap.accounts[addr].length(),
				testCase.expected.enqueued,
			)

			// promoted
			assert.Equal(t,
				pool.promoted.length(),
				testCase.expected.promoted,
			)
		})
	}
}

func TestExecuteTransactionsMultipleAccounts(t *testing.T) {
	// same as above, but more realistic
}
