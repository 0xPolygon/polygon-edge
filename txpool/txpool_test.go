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

func newTestPool() (*TxPool, error) {
	return NewTxPool(
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
}

// expected pool state for some account after test run
type result struct {
	enqueued int // number of enqueued txs
	promoted int // number of promoted txs
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
				Gas:      1,
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			pool, err := newTestPool()
			assert.NoError(t, err)
			pool.EnableDev()
			pool.AddSigner(crypto.NewEIP155Signer(100))

			/* special error setups */
			switch testCase.expectedError {
			case ErrTxPoolOverflow:
				pool.gauge.increase(defaultMaxSlots)
			case ErrAlreadyKnown:
				// TODO
			case ErrNonEncryptedTx:
				pool.dev = false
			case ErrInvalidAccountState:
				pool.store = faultyMockStore{}
			}

			assert.ErrorIs(t,
				pool.addTx(testCase.tx),
				testCase.expectedError,
			)
		})
	}
}

func TestHandleAddRequest(t *testing.T) {
	testCases := []struct {
		name      string
		txNonce   uint64
		nextNonce uint64
		expected  handlerEvent
	}{
		{
			name:      "signal promotion",
			txNonce:   1,
			nextNonce: 1,
			expected:  signalPromotion,
		},
		{
			name:      "enqueue higher nonce",
			txNonce:   5,
			nextNonce: 2,
			expected:  txEnqueued,
		},
		{
			name:      "reject low nonce",
			txNonce:   3,
			nextNonce: 6,
			expected:  txRejected,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			// 1. create pool
			pool, err := newTestPool()
			assert.NoError(t, err)
			pool.EnableDev()
			pool.AddSigner(&mockSigner{})
			from := types.Address{0x1}

			// 2. prepare pool
			pool.createAccountOnce(from)
			pool.nextNonces.store(from, testCase.nextNonce)

			// 3. handle request
			done := pool.handleAddRequest(addRequest{
				tx: &types.Transaction{
					From:     from,
					Nonce:    testCase.txNonce,
					Gas:      validGasLimit,
					GasPrice: big.NewInt(1),
					Value:    big.NewInt(0),
				},
			})

			// wait for handler to finish
			res := <-done
			assert.Equal(t, testCase.expected, res)
		})
	}
}

func TestMultipleAddRequestHandlers(t *testing.T) {
	type addTx struct {
		nonce    uint64
		expected handlerEvent
	}

	testCases := []struct {
		name      string
		nextNonce uint64
		txs       []addTx
	}{
		{
			name:      "signal promote 1, reject 3, enqueue 3",
			nextNonce: 3,
			txs: []addTx{
				{
					nonce:    0,
					expected: txRejected,
				},
				{
					nonce:    1,
					expected: txRejected,
				},
				{
					nonce:    2,
					expected: txRejected,
				},
				{
					nonce:    3,
					expected: signalPromotion,
				},
				{
					nonce:    9,
					expected: txEnqueued,
				},
				{
					nonce:    8,
					expected: txEnqueued,
				},
				{
					nonce:    10,
					expected: txEnqueued,
				},
			},
		},
		{
			name:      "reject all",
			nextNonce: 5,
			txs: []addTx{
				{
					nonce:    2,
					expected: txRejected,
				},
				{
					nonce:    1,
					expected: txRejected,
				},
				{
					nonce:    4,
					expected: txRejected,
				},
				{
					nonce:    3,
					expected: txRejected,
				},
			},
		},
		{
			name:      "last added tx triggers promotion",
			nextNonce: 0,
			txs: []addTx{
				{
					nonce:    1,
					expected: txEnqueued,
				},
				{
					nonce:    2,
					expected: txEnqueued,
				},
				{
					nonce:    3,
					expected: txEnqueued,
				},
				{
					nonce:    4,
					expected: txEnqueued,
				},
				{
					nonce:    5,
					expected: txEnqueued,
				},
				{
					nonce:    0,
					expected: signalPromotion,
				},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			// 1. create pool
			pool, err := newTestPool()
			assert.NoError(t, err)
			pool.EnableDev()
			pool.AddSigner(&mockSigner{})
			from := types.Address{0x1}

			// 2. set nonce map to expected
			pool.nextNonces.store(from, testCase.nextNonce)

			// 3. handle requests
			for _, tx := range testCase.txs {
				done := pool.handleAddRequest(addRequest{
					tx: &types.Transaction{
						From:     from,
						Nonce:    tx.nonce,
						Gas:      validGasLimit,
						GasPrice: big.NewInt(1),
						Value:    big.NewInt(0),
					},
				})

				// wait for handler to finish
				res := <-done
				assert.Equal(t, tx.expected, res)
			}
		})
	}
}

func TestPromoteRequestHandler(t *testing.T) {
	testCases := []struct {
		name         string
		enqueued     []uint64 // tx nonce
		nextNonce    uint64
		promoteEvent handlerEvent
		expected     result
	}{
		{
			name: "promote all",
			enqueued: []uint64{
				0, 1, 2, 3, 4, 5, 6, 7,
			},
			nextNonce:    0,
			promoteEvent: accountPromoted,
			expected: result{
				enqueued: 0,
				promoted: 8,
			},
		},
		{
			name: "promote 4 out of 8",
			enqueued: []uint64{
				2, 3, 4, 5, 8, 9, 10, 11,
			},
			nextNonce:    2,
			promoteEvent: accountPromoted,
			expected: result{
				enqueued: 4,
				promoted: 4,
			},
		},
		{
			name: "promote none",
			enqueued: []uint64{
				5, 6, 7, 8, 9,
			},
			nextNonce:    3,
			promoteEvent: nothingToPromote,
			expected: result{
				enqueued: 5,
				promoted: 0,
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			// 1. create pool
			pool, err := newTestPool()
			assert.NoError(t, err)
			pool.EnableDev()
			pool.AddSigner(&mockSigner{})
			from := types.Address{0x1}

			// 2. prepare account queue
			pool.createAccountOnce(from)
			pool.nextNonces.store(from, testCase.nextNonce)
			queue := pool.enqueued[from]
			for _, nonce := range testCase.enqueued {
				queue.push(&types.Transaction{
					From:     from,
					Nonce:    nonce,
					Gas:      validGasLimit,
					GasPrice: big.NewInt(1),
					Value:    big.NewInt(0),
				})
			}

			assert.Equal(t,
				queue.length(),
				len(testCase.enqueued),
			)

			// 3. handle promote
			done := pool.handlePromoteRequest(promoteRequest{account: from})
			res := <-done

			// 4. assert

			// done event
			assert.Equal(t,
				testCase.promoteEvent,
				res,
			)

			// enqueued
			assert.Equal(t,
				testCase.expected.enqueued,
				pool.enqueued[from].length(),
			)

			// prmoted
			assert.Equal(t,
				testCase.expected.promoted,
				pool.promoted.length(),
			)
		})
	}
}

func TestResetRequestHandler(t *testing.T) {
	type initialState struct {
		enqueued map[types.Address][]uint64
		promoted map[types.Address][]uint64
	}

	from1 := types.Address{0x1}
	// from2 := types.Address{0x2}
	// from3 := types.Address{0x3}

	testCases := []struct {
		name        string
		accounts    []types.Address
		resetNonces map[types.Address]uint64
		before      initialState
		after       map[types.Address]result
	}{
		{
			name: "Reset one account",

			accounts: []types.Address{
				from1,
			},

			resetNonces: map[types.Address]uint64{
				from1: 6,
			},

			before: initialState{
				enqueued: map[types.Address][]uint64{
					from1: {6, 7, 8},
				},
				promoted: map[types.Address][]uint64{
					from1: {1, 2, 3},
				},
			},

			after: map[types.Address]result{
				from1: {
					enqueued: 2,
					promoted: 0,
				},
			},
		},
		// {
		// 	name: "Reset two accounts",
		// },
		// {
		// 	name: "Reset all accounts",
		// },
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			// 1. create pool
			pool, err := newTestPool()
			assert.NoError(t, err)
			pool.EnableDev()
			pool.AddSigner(&mockSigner{})

			// 2. prepare initial state
			for addr, nonces := range testCase.before.enqueued {
				pool.createAccountOnce(addr)
				for _, nonce := range nonces {
					// insert enqueued
					queue := pool.enqueued[addr]
					queue.push(&types.Transaction{
						From:     addr,
						Nonce:    nonce,
						Gas:      validGasLimit,
						GasPrice: big.NewInt(1),
						Value:    big.NewInt(0),
					})
				}
			}

			for addr, nonces := range testCase.before.promoted {
				for _, nonce := range nonces {
					// insert promoted
					pool.promoted.push(&types.Transaction{
						From:     addr,
						Nonce:    nonce,
						Gas:      validGasLimit,
						GasPrice: big.NewInt(1),
						Value:    big.NewInt(0),
					})
				}
			}

			// 3. handle reset
			done := pool.handleResetRequest(resetRequest{testCase.resetNonces})
			res := <-done

			// 4. assert
			assert.Equal(t,
				resetDone,
				res,
			)

			totalPromoted := 0
			for addr, result := range testCase.after {
				totalPromoted += result.promoted
				// enqueued
				assert.Equal(t,
					result.enqueued,
					pool.enqueued[addr].length(),
				)
			}

			// promoted
			assert.Equal(t,
				totalPromoted,
				pool.promoted.length(),
			)
		})
	}
}

// TODO
func TestRollbackRequestHandler(t *testing.T) {

}

// helper object to simulate error events
// the actual callback would return
type writeTx struct {
	raw    *types.Transaction
	status WriteTxStatus
}

// mock transition write callback
func mockWrite(tx writeTx) WriteTxStatus {
	return tx.status
}

// TODO
func TestWriteTransactions(t *testing.T) {

}
