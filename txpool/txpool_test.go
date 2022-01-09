package txpool

import (
	"crypto/rand"
	"math/big"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-sdk/chain"
	"github.com/0xPolygon/polygon-sdk/crypto"
	"github.com/0xPolygon/polygon-sdk/helper/tests"
	"github.com/0xPolygon/polygon-sdk/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
)

const (
	defaultPriceLimit uint64 = 1
	defaultMaxSlots   uint64 = 4096
	validGasLimit     uint64 = 10000000
)

var (
	forks = &chain.Forks{
		Homestead: chain.NewFork(0),
		Istanbul:  chain.NewFork(0),
	}

	nilMetrics = NilMetrics()
)

// addresses used in tests
var (
	addr1  = types.Address{0x1}
	addr2  = types.Address{0x2}
	addr3  = types.Address{0x3}
	addr4  = types.Address{0x4}
	addr5  = types.Address{0x5}
	addr6  = types.Address{0x6}
	addr7  = types.Address{0x7}
	addr8  = types.Address{0x8}
	addr9  = types.Address{0x9}
	addr10 = types.Address{0x10}
)

// returns a new valid tx of slots size with the given nonce
func newDummyTx(addr types.Address, nonce, slots uint64) *types.Transaction {
	// base field should take 1 slot at least
	size := txSlotSize * (slots - 1)
	if size <= 0 {
		size = 1
	}

	input := make([]byte, size)
	if _, err := rand.Read(input); err != nil {
		return nil
	}

	return &types.Transaction{
		From:     addr,
		Nonce:    nonce,
		Value:    big.NewInt(1),
		GasPrice: big.NewInt(0).SetUint64(defaultPriceLimit),
		Gas:      validGasLimit,
		Input:    input,
	}
}

// returns a new txpool with default test config
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

type accountState struct {
	enqueued, promoted uint64
}

type result struct {
	accounts map[types.Address]accountState
	slots    uint64
}

/* Singe account cases (unit tests) */

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
				From:     addr1,
				Value:    big.NewInt(-5),
				GasPrice: big.NewInt(1),
				Gas:      validGasLimit,
			},
		},
		{
			name:          "ErrNonEncryptedTx",
			expectedError: ErrNonEncryptedTx,
			tx: &types.Transaction{
				From:     addr1,
				Value:    big.NewInt(1),
				GasPrice: big.NewInt(1),
				Gas:      validGasLimit,
			},
		},
		{
			name:          "ErrInvalidSender",
			expectedError: ErrInvalidSender,
			tx: &types.Transaction{
				From:     types.ZeroAddress,
				Value:    big.NewInt(1),
				GasPrice: big.NewInt(1),
				Gas:      validGasLimit,
			},
		},
		{
			name:          "ErrInvalidAccountState",
			expectedError: ErrInvalidAccountState,
			tx: &types.Transaction{
				From:     addr1,
				Value:    big.NewInt(1),
				GasPrice: big.NewInt(1),
				Gas:      validGasLimit,
			},
		},
		{
			name:          "ErrTxPoolOverflow",
			expectedError: ErrTxPoolOverflow,
			tx: &types.Transaction{
				From:     addr1,
				Value:    big.NewInt(1),
				GasPrice: big.NewInt(1),
				Gas:      validGasLimit,
			},
		},
		{
			name:          "ErrIntrinsicGas",
			expectedError: ErrIntrinsicGas,
			tx: &types.Transaction{
				From:     addr1,
				Value:    big.NewInt(1),
				GasPrice: big.NewInt(1),
				Gas:      1,
			},
		},
		{
			name:          "ErrAlreadyKnown",
			expectedError: ErrAlreadyKnown,
			tx: &types.Transaction{
				From:     addr1,
				Value:    big.NewInt(1),
				GasPrice: big.NewInt(1),
				Gas:      validGasLimit,
			},
		},
		{
			name:          "ErrOversizedData",
			expectedError: ErrOversizedData,
			tx: &types.Transaction{
				From:     addr1,
				Value:    big.NewInt(1),
				GasPrice: big.NewInt(1),
				Gas:      validGasLimit,
			},
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			pool, err := newTestPool()
			assert.NoError(t, err)
			pool.EnableDev()
			pool.AddSigner(crypto.NewEIP155Signer(100))

			/* special error setups */
			switch test.expectedError {
			case ErrTxPoolOverflow:
				pool.gauge.increase(defaultMaxSlots)
			case ErrNonEncryptedTx:
				pool.dev = false
			case ErrAlreadyKnown:
				// send the tx beforehand
				go pool.addTx(local, test.tx)
				go pool.handleAddRequest(<-pool.addReqCh)
				<-pool.promoteReqCh
			case ErrInvalidAccountState:
				pool.store = faultyMockStore{}
			case ErrOversizedData:
				data := make([]byte, 989898)
				rand.Read(data)
				test.tx.Input = data
			}

			assert.ErrorIs(t,
				test.expectedError,
				pool.addTx(local, test.tx),
			)
		})
	}
}

func TestAddHandler(t *testing.T) {
	t.Run("enqueue new tx with higher nonce", func(t *testing.T) {
		pool, err := newTestPool()
		assert.NoError(t, err)
		pool.AddSigner(&mockSigner{})
		pool.EnableDev()

		// send higher nonce tx
		go pool.addTx(local, newDummyTx(addr1, 10, 1)) // 10 > 0
		pool.handleAddRequest(<-pool.addReqCh)

		assert.Equal(t, uint64(1), pool.gauge.read())
		assert.Equal(t, uint64(1), pool.accounts.from(addr1).enqueued.length())
	})

	t.Run("reject new tx with low nonce", func(t *testing.T) {
		pool, err := newTestPool()
		assert.NoError(t, err)
		pool.AddSigner(&mockSigner{})
		pool.EnableDev()

		{ // set up prestate
			acc := pool.createAccountOnce(addr1)
			acc.setNonce(20)
		}

		// send tx
		go pool.addTx(local, newDummyTx(addr1, 10, 1)) // 10 < 20
		pool.handleAddRequest(<-pool.addReqCh)

		assert.Equal(t, uint64(0), pool.gauge.read())
		assert.Equal(t, uint64(0), pool.accounts.from(addr1).enqueued.length())
	})

	t.Run("signal promotion for new tx with expected nonce", func(t *testing.T) {
		pool, err := newTestPool()
		assert.NoError(t, err)
		pool.AddSigner(&mockSigner{})
		pool.EnableDev()

		// send tx
		go pool.addTx(local, newDummyTx(addr1, 0, 1)) // 0 == 0
		go pool.handleAddRequest(<-pool.addReqCh)

		// catch pending promotion
		<-pool.promoteReqCh

		assert.Equal(t, uint64(1), pool.gauge.read())
		assert.Equal(t, uint64(1), pool.accounts.from(addr1).enqueued.length())
		assert.Equal(t, uint64(0), pool.accounts.from(addr1).promoted.length())
	})

	t.Run("signal promotion for demoted tx with low nonce", func(t *testing.T) {
		pool, err := newTestPool()
		assert.NoError(t, err)
		pool.AddSigner(&mockSigner{})
		pool.EnableDev()

		{ // set up prestate
			acc := pool.createAccountOnce(addr1)
			acc.setNonce(5)
		}

		// send demoted (recovered and promotable)
		go pool.handleAddRequest(addRequest{
			tx:      newDummyTx(addr1, 3, 1), // 3 < 5
			demoted: true,
		})

		// catch pending promotion
		<-pool.promoteReqCh

		assert.Equal(t, uint64(1), pool.accounts.from(addr1).enqueued.length())
		assert.Equal(t, uint64(0), pool.accounts.from(addr1).promoted.length())
	})

	t.Run("enqueue demoted tx with higher nonce", func(t *testing.T) {
		pool, err := newTestPool()
		assert.NoError(t, err)
		pool.AddSigner(&mockSigner{})
		pool.EnableDev()

		{ // set up prestate
			acc := pool.createAccountOnce(addr1)
			acc.setNonce(5)
		}

		// send demoted (recovered but not promotable)
		pool.handleAddRequest(addRequest{
			tx:      newDummyTx(addr1, 8, 1), // 8 > 5
			demoted: true,
		})

		assert.Equal(t, uint64(1), pool.accounts.from(addr1).enqueued.length())
		assert.Equal(t, uint64(0), pool.accounts.from(addr1).promoted.length())
	})
}

func TestPromoteHandler(t *testing.T) {
	t.Run("nothing to promote", func(t *testing.T) {
		/* This test demonstrates that if some promotion handler
		got its job done by a previous one, it will not perform any logic
		by doing an early return. */
		pool, err := newTestPool()
		assert.NoError(t, err)
		pool.AddSigner(&mockSigner{})
		pool.EnableDev()

		// fake a promotion signal
		signalPromotion := func() {
			pool.promoteReqCh <- promoteRequest{account: addr1}
		}

		// fresh account (queues are empty)
		acc := pool.createAccountOnce(addr1)
		acc.setNonce(7)

		// fake a promotion
		go signalPromotion()
		pool.handlePromoteRequest(<-pool.promoteReqCh)
		assert.Equal(t, uint64(0), pool.accounts.from(addr1).enqueued.length())
		assert.Equal(t, uint64(0), pool.accounts.from(addr1).promoted.length())

		// enqueue higher nonce tx
		go pool.addTx(local, newDummyTx(addr1, 10, 1))
		pool.handleAddRequest(<-pool.addReqCh)
		assert.Equal(t, uint64(1), pool.accounts.from(addr1).enqueued.length())
		assert.Equal(t, uint64(0), pool.accounts.from(addr1).promoted.length())

		// fake a promotion
		go signalPromotion()
		pool.handlePromoteRequest(<-pool.promoteReqCh)
		assert.Equal(t, uint64(1), pool.accounts.from(addr1).enqueued.length())
		assert.Equal(t, uint64(0), pool.accounts.from(addr1).promoted.length())
	})

	t.Run("promote one tx", func(t *testing.T) {
		pool, err := newTestPool()
		assert.NoError(t, err)
		pool.AddSigner(&mockSigner{})
		pool.EnableDev()

		go pool.addTx(local, newDummyTx(addr1, 0, 1))
		go pool.handleAddRequest(<-pool.addReqCh)

		// tx enqueued -> promotion signaled
		pool.handlePromoteRequest(<-pool.promoteReqCh)

		assert.Equal(t, uint64(1), pool.gauge.read())
		assert.Equal(t, uint64(1), pool.accounts.from(addr1).getNonce())

		assert.Equal(t, uint64(0), pool.accounts.from(addr1).enqueued.length())
		assert.Equal(t, uint64(1), pool.accounts.from(addr1).promoted.length())
	})

	t.Run("promote several txs", func(t *testing.T) {
		/* This example illustrates the flexibility of the handlers:
		One promotion handler can be executed at any time after it
		was invoked (when the runtime decides), resulting in promotion
		of several enqueued txs. */
		pool, err := newTestPool()
		assert.NoError(t, err)
		pool.AddSigner(&mockSigner{})
		pool.EnableDev()

		// send the first (expected) tx -> signals promotion
		go pool.addTx(local, newDummyTx(addr1, 0, 1)) // 0 == 0
		go pool.handleAddRequest(<-pool.addReqCh)

		// save the promotion handler
		req := <-pool.promoteReqCh

		// send the remaining txs (all will be enqueued)
		for nonce := uint64(1); nonce < 10; nonce++ {
			go pool.addTx(local, newDummyTx(addr1, nonce, 1))
			pool.handleAddRequest(<-pool.addReqCh)
		}

		// verify all 10 are enqueued
		assert.Equal(t, uint64(10), pool.accounts.from(addr1).enqueued.length())
		assert.Equal(t, uint64(0), pool.accounts.from(addr1).promoted.length())

		// execute the promotion handler
		pool.handlePromoteRequest(req)

		assert.Equal(t, uint64(10), pool.gauge.read())
		assert.Equal(t, uint64(10), pool.accounts.from(addr1).getNonce())

		assert.Equal(t, uint64(0), pool.accounts.from(addr1).enqueued.length())
		assert.Equal(t, uint64(10), pool.accounts.from(addr1).promoted.length())
	})

	t.Run("one tx -> one promotion", func(t *testing.T) {
		/* In this scenario, each received tx will be instantly promoted.
		All txs are sent in the order of expected nonce. */
		pool, err := newTestPool()
		assert.NoError(t, err)
		pool.AddSigner(&mockSigner{})
		pool.EnableDev()

		for nonce := uint64(0); nonce < 20; nonce++ {
			go pool.addTx(local, newDummyTx(addr1, nonce, 1))
			go pool.handleAddRequest(<-pool.addReqCh)
			pool.handlePromoteRequest(<-pool.promoteReqCh)
		}

		assert.Equal(t, uint64(20), pool.gauge.read())
		assert.Equal(t, uint64(20), pool.accounts.from(addr1).getNonce())

		assert.Equal(t, uint64(0), pool.accounts.from(addr1).enqueued.length())
		assert.Equal(t, uint64(20), pool.accounts.from(addr1).promoted.length())
	})

	t.Run("promote demoted tx with low nonce", func(t *testing.T) {
		pool, err := newTestPool()
		assert.NoError(t, err)
		pool.AddSigner(&mockSigner{})
		pool.EnableDev()

		{ // setup prestate
			acc := pool.createAccountOnce(addr1)
			acc.setNonce(7)
		}

		// send recovered tx
		go pool.handleAddRequest(addRequest{
			tx:      newDummyTx(addr1, 4, 1),
			demoted: true,
		})

		// promote
		pool.handlePromoteRequest(<-pool.promoteReqCh)

		assert.Equal(t, uint64(0), pool.accounts.from(addr1).enqueued.length())
		assert.Equal(t, uint64(1), pool.accounts.from(addr1).promoted.length())
	})
}

func TestResetAccount(t *testing.T) {
	t.Run("reset promoted", func(t *testing.T) {
		testCases := []struct {
			name     string
			txs      transactions
			newNonce uint64
			expected result
		}{
			{
				name: "prune all txs",
				txs: transactions{
					newDummyTx(addr1, 0, 1),
					newDummyTx(addr1, 1, 1),
					newDummyTx(addr1, 2, 1),
					newDummyTx(addr1, 3, 1),
					newDummyTx(addr1, 4, 1),
				},
				newNonce: 5,
				expected: result{
					slots: 0,
					accounts: map[types.Address]accountState{
						addr1: {
							promoted: 0,
						},
					},
				},
			},
			{
				name: "prune no txs",
				txs: transactions{
					newDummyTx(addr1, 2, 1),
					newDummyTx(addr1, 3, 1),
					newDummyTx(addr1, 4, 1),
				},
				newNonce: 1,
				expected: result{
					slots: 3,
					accounts: map[types.Address]accountState{
						addr1: {
							promoted: 3,
						},
					},
				},
			},
			{
				name: "prune some txs",
				txs: transactions{
					newDummyTx(addr1, 7, 1),
					newDummyTx(addr1, 8, 1),
					newDummyTx(addr1, 9, 1),
				},
				newNonce: 8,
				expected: result{
					slots: 2,
					accounts: map[types.Address]accountState{
						addr1: {
							promoted: 2,
						},
					},
				},
			},
		}
		for _, test := range testCases {
			t.Run(test.name, func(t *testing.T) {
				pool, err := newTestPool()
				assert.NoError(t, err)
				pool.AddSigner(&mockSigner{})
				pool.EnableDev()

				{ // setup prestate
					acc := pool.createAccountOnce(addr1)
					acc.setNonce(test.txs[0].Nonce)

					go pool.addTx(local, test.txs[0])
					go pool.handleAddRequest(<-pool.addReqCh)

					// save the promotion
					req := <-pool.promoteReqCh

					// enqueue remaining
					for i, tx := range test.txs {
						if i == 0 {
							// first was handled
							continue
						}
						go pool.addTx(local, tx)
						pool.handleAddRequest(<-pool.addReqCh)
					}

					pool.handlePromoteRequest(req)
					assert.Equal(t, uint64(0), pool.accounts.from(addr1).enqueued.length())
					assert.Equal(t, uint64(len(test.txs)), pool.accounts.from(addr1).promoted.length())
				}

				pool.resetAccount(addr1, test.newNonce)

				assert.Equal(t, test.expected.slots, pool.gauge.read())
				assert.Equal(t, // enqueued
					test.expected.accounts[addr1].enqueued,
					pool.accounts.from(addr1).enqueued.length())
				assert.Equal(t, // promoted
					test.expected.accounts[addr1].promoted,
					pool.accounts.from(addr1).promoted.length())
			})
		}
	})

	t.Run("reset enqueued", func(t *testing.T) {
		testCases := []struct {
			name     string
			txs      transactions
			newNonce uint64
			expected result
			signal   bool // flag indicating whether reset will cause a promotion
		}{
			{
				name: "prune all txs",
				txs: transactions{
					newDummyTx(addr1, 5, 1),
					newDummyTx(addr1, 6, 1),
					newDummyTx(addr1, 7, 1),
					newDummyTx(addr1, 8, 1),
				},
				newNonce: 10,
				expected: result{
					slots: 0,
					accounts: map[types.Address]accountState{
						addr1: {
							enqueued: 0,
						},
					},
				},
			},
			{
				name: "prune no txs",
				txs: transactions{
					newDummyTx(addr1, 2, 1),
					newDummyTx(addr1, 3, 1),
					newDummyTx(addr1, 4, 1),
				},
				newNonce: 1,
				expected: result{
					slots: 3,
					accounts: map[types.Address]accountState{
						addr1: {
							enqueued: 3,
						},
					},
				},
			},
			{
				name: "prune some txs",
				txs: transactions{
					newDummyTx(addr1, 4, 1),
					newDummyTx(addr1, 5, 1),
					newDummyTx(addr1, 8, 1),
					newDummyTx(addr1, 9, 1),
				},
				newNonce: 6,
				expected: result{
					slots: 2,
					accounts: map[types.Address]accountState{
						addr1: {
							enqueued: 2,
						},
					},
				},
			},
			{
				name:   "prune signals promotion",
				signal: true,
				txs: transactions{
					newDummyTx(addr1, 8, 1),
					newDummyTx(addr1, 9, 1),
					newDummyTx(addr1, 10, 1),
				},
				newNonce: 9,
				expected: result{
					slots: 2,
					accounts: map[types.Address]accountState{
						addr1: {
							enqueued: 0,
							promoted: 2,
						},
					},
				},
			},
		}

		for _, test := range testCases {
			t.Run(test.name, func(t *testing.T) {
				pool, err := newTestPool()
				assert.NoError(t, err)
				pool.AddSigner(&mockSigner{})
				pool.EnableDev()

				{ // setup prestate
					for _, tx := range test.txs {
						go pool.addTx(local, tx)
						pool.handleAddRequest(<-pool.addReqCh)
					}

					assert.Equal(t, uint64(len(test.txs)), pool.accounts.from(addr1).enqueued.length())
					assert.Equal(t, uint64(0), pool.accounts.from(addr1).promoted.length())
				}

				if test.signal {
					go pool.resetAccount(addr1, test.newNonce)
					pool.handlePromoteRequest(<-pool.promoteReqCh)
				} else {
					pool.resetAccount(addr1, test.newNonce)
				}

				assert.Equal(t, test.expected.slots, pool.gauge.read())
				assert.Equal(t, // enqueued
					test.expected.accounts[addr1].enqueued,
					pool.accounts.from(addr1).enqueued.length())
				assert.Equal(t, // promoted
					test.expected.accounts[addr1].promoted,
					pool.accounts.from(addr1).promoted.length())
			})
		}
	})

	t.Run("reset all", func(t *testing.T) {
		testCases := []struct {
			name     string
			txs      transactions
			newNonce uint64
			expected result
			signal   bool // flag indicating whether reset will cause a promotion
		}{
			{
				name: "prune all txs",
				txs: transactions{
					// promoted
					newDummyTx(addr1, 0, 1),
					newDummyTx(addr1, 1, 1),
					newDummyTx(addr1, 2, 1),
					newDummyTx(addr1, 3, 1),
					// enqueued
					newDummyTx(addr1, 5, 1),
					newDummyTx(addr1, 6, 1),
					newDummyTx(addr1, 8, 1),
				},
				newNonce: 10,
				expected: result{
					slots: 0,
					accounts: map[types.Address]accountState{
						addr1: {
							enqueued: 0,
							promoted: 0,
						},
					},
				},
			},
			{
				name: "prune no txs",
				txs: transactions{
					// promoted
					newDummyTx(addr1, 5, 1),
					newDummyTx(addr1, 6, 1),
					// enqueued
					newDummyTx(addr1, 9, 1),
					newDummyTx(addr1, 10, 1),
				},
				newNonce: 3,
				expected: result{
					slots: 4,
					accounts: map[types.Address]accountState{
						addr1: {
							enqueued: 2,
							promoted: 2,
						},
					},
				},
			},
			{
				name: "prune some txs",
				txs: transactions{
					// promoted
					newDummyTx(addr1, 1, 1),
					newDummyTx(addr1, 2, 1),
					newDummyTx(addr1, 3, 1),
					// enqueued
					newDummyTx(addr1, 5, 1),
					newDummyTx(addr1, 8, 1),
					newDummyTx(addr1, 9, 1),
				},
				newNonce: 6,
				expected: result{
					slots: 2,
					accounts: map[types.Address]accountState{
						addr1: {
							enqueued: 2,
							promoted: 0,
						},
					},
				},
			},
			{
				name:   "prune signals promotion",
				signal: true,
				txs: transactions{
					// promoted
					newDummyTx(addr1, 2, 1),
					newDummyTx(addr1, 3, 1),
					newDummyTx(addr1, 4, 1),
					newDummyTx(addr1, 5, 1),
					// enqueued
					newDummyTx(addr1, 8, 1),
					newDummyTx(addr1, 9, 1),
					newDummyTx(addr1, 10, 1),
				},
				newNonce: 8,
				expected: result{
					slots: 3,
					accounts: map[types.Address]accountState{
						addr1: {
							enqueued: 0,
							promoted: 3,
						},
					},
				},
			},
		}

		for _, test := range testCases {
			t.Run(test.name, func(t *testing.T) {
				pool, err := newTestPool()
				assert.NoError(t, err)
				pool.AddSigner(&mockSigner{})
				pool.EnableDev()

				{ // setup prestate
					acc := pool.createAccountOnce(addr1)
					acc.setNonce(test.txs[0].Nonce)

					go pool.addTx(local, test.txs[0])
					go pool.handleAddRequest(<-pool.addReqCh)

					// save the promotion
					req := <-pool.promoteReqCh

					// enqueue remaining
					for i, tx := range test.txs {
						if i == 0 {
							// first was handled
							continue
						}
						go pool.addTx(local, tx)
						pool.handleAddRequest(<-pool.addReqCh)
					}

					pool.handlePromoteRequest(req)
				}

				if test.signal {
					go pool.resetAccount(addr1, test.newNonce)
					pool.handlePromoteRequest(<-pool.promoteReqCh)
				} else {
					pool.resetAccount(addr1, test.newNonce)
				}

				assert.Equal(t, test.expected.slots, pool.gauge.read())
				assert.Equal(t, // enqueued
					test.expected.accounts[addr1].enqueued,
					pool.accounts.from(addr1).enqueued.length())
				assert.Equal(t, // promoted
					test.expected.accounts[addr1].promoted,
					pool.accounts.from(addr1).promoted.length())
			})
		}
	})
}

func TestPop(t *testing.T) {
	pool, err := newTestPool()
	assert.NoError(t, err)
	pool.AddSigner(&mockSigner{})
	pool.EnableDev()

	// send 1 tx and promote it
	go pool.addTx(local, newDummyTx(addr1, 0, 1))
	go pool.handleAddRequest(<-pool.addReqCh)
	pool.handlePromoteRequest(<-pool.promoteReqCh)

	assert.Equal(t, uint64(1), pool.gauge.read())
	assert.Equal(t, uint64(1), pool.accounts.from(addr1).promoted.length())

	// pop the tx
	pool.Prepare()
	tx := pool.Peek()
	pool.Pop(tx)

	assert.Equal(t, uint64(0), pool.gauge.read())
	assert.Equal(t, uint64(0), pool.accounts.from(addr1).promoted.length())
}
func TestDrop(t *testing.T) {
	pool, err := newTestPool()
	assert.NoError(t, err)
	pool.AddSigner(&mockSigner{})
	pool.EnableDev()

	// send 1 tx and promote it
	go pool.addTx(local, newDummyTx(addr1, 0, 1))
	go pool.handleAddRequest(<-pool.addReqCh)
	pool.handlePromoteRequest(<-pool.promoteReqCh)

	assert.Equal(t, uint64(1), pool.gauge.read())
	assert.Equal(t, uint64(1), pool.accounts.from(addr1).getNonce())
	assert.Equal(t, uint64(1), pool.accounts.from(addr1).promoted.length())

	// pop the tx
	pool.Prepare()
	tx := pool.Peek()
	pool.Drop(tx)

	assert.Equal(t, uint64(0), pool.gauge.read())
	assert.Equal(t, uint64(0), pool.accounts.from(addr1).getNonce())
	assert.Equal(t, uint64(0), pool.accounts.from(addr1).promoted.length())
}
func TestDemote(t *testing.T) {
	t.Run("demote will promote", func(t *testing.T) {
		pool, err := newTestPool()
		assert.NoError(t, err)
		pool.AddSigner(&mockSigner{})
		pool.EnableDev()

		// send 1st tx
		go pool.addTx(local, newDummyTx(addr1, 0, 1))
		go pool.handleAddRequest(<-pool.addReqCh)

		// save the promotion for later
		prom := <-pool.promoteReqCh

		// send 2nd tx
		go pool.addTx(local, newDummyTx(addr1, 1, 1))
		pool.handleAddRequest(<-pool.addReqCh)

		// promote
		pool.handlePromoteRequest(prom)

		assert.Equal(t, uint64(2), pool.gauge.read())
		assert.Equal(t, uint64(2), pool.accounts.from(addr1).getNonce())
		assert.Equal(t, uint64(2), pool.accounts.from(addr1).promoted.length())

		pool.Prepare()

		// process 1st tx
		tx := pool.Peek()
		pool.Pop(tx)

		assert.Equal(t, uint64(1), pool.gauge.read())
		assert.Equal(t, uint64(1), pool.accounts.from(addr1).promoted.length())

		/* demote 2nd tx */
		tx = pool.Peek()

		// save the add request
		// originating in Demote
		var addReq addRequest
		go func() {
			addReq = <-pool.addReqCh
		}()

		pool.Demote(tx)

		// handle add request
		go pool.handleAddRequest(addReq)
		pool.handlePromoteRequest(<-pool.promoteReqCh)

		assert.Equal(t, uint64(1), pool.gauge.read())
		assert.Equal(t, uint64(0), pool.accounts.from(addr1).enqueued.length())
		assert.Equal(t, uint64(1), pool.accounts.from(addr1).promoted.length())
	})

	t.Run("demote will enqueue", func(t *testing.T) {
		pool, err := newTestPool()
		assert.NoError(t, err)
		pool.AddSigner(&mockSigner{})
		pool.EnableDev()

		// send 2 txs and promote them
		go pool.addTx(local, newDummyTx(addr1, 0, 1))
		go pool.handleAddRequest(<-pool.addReqCh)
		prom := <-pool.promoteReqCh
		go pool.addTx(local, newDummyTx(addr1, 1, 1))
		pool.handleAddRequest(<-pool.addReqCh)
		pool.handlePromoteRequest(prom)

		assert.Equal(t, uint64(2), pool.gauge.read())
		assert.Equal(t, uint64(2), pool.accounts.from(addr1).getNonce())
		assert.Equal(t, uint64(2), pool.accounts.from(addr1).promoted.length())

		pool.Prepare()

		// drop first tx
		tx := pool.Peek()
		pool.Drop(tx) // this will rollback nonce

		assert.Equal(t, uint64(1), pool.gauge.read())
		assert.Equal(t, uint64(0), pool.accounts.from(addr1).getNonce())
		assert.Equal(t, uint64(1), pool.accounts.from(addr1).promoted.length())

		// demote second tx
		tx = pool.Peek()
		// save the add request
		// originating in Demote
		var addReq addRequest
		go func() {
			addReq = <-pool.addReqCh
		}()

		pool.Demote(tx)

		// enqueue demoted
		pool.handleAddRequest(addReq)

		assert.Equal(t, uint64(1), pool.gauge.read())
		assert.Equal(t, uint64(1), pool.accounts.from(addr1).enqueued.length())
		assert.Equal(t, uint64(0), pool.accounts.from(addr1).promoted.length())
	})
}

/* "Integrated" tests */

// The following tests ensure that the pool's inner event loop
// is handling requests correctly, meaning that we do not have
// to assume its role (like in previous unit tests) and
// perform dispatching/handling on our own.
//
// To determine when the pool is done handling requests
// waitUntilDone(done chan) breaks out of its polling loop
// when there's no more activity on the done channel,
// previously provided by startTestMode()

// Starts the pool's event loop and returns a channel
// that receives a notification every time a request
// is handled.
func (p *TxPool) startTestMode() <-chan struct{} {
	done := make(chan struct{})

	go func() {
		for {
			select {
			case req := <-p.addReqCh:
				go func() {
					p.handleAddRequest(req)
					done <- struct{}{}
				}()
			case req := <-p.promoteReqCh:
				go func() {
					p.handlePromoteRequest(req)
					done <- struct{}{}
				}()
			}
		}
	}()

	return done
}

// Listens for activity on the pool's event loop.
// Assumes the pool is finished if the timer expires.
// Timer is reset each time a request is handled.
func waitUntilDone(done <-chan struct{}) {
	for {
		select {
		case <-done:
		case <-time.After(10 * time.Millisecond):
			return
		}
	}
}

func TestAddTx100(t *testing.T) {
	t.Run("send 100 transactions", func(t *testing.T) {
		pool, err := newTestPool()
		assert.NoError(t, err)
		pool.AddSigner(&mockSigner{})
		pool.EnableDev()

		// start the main loop
		done := pool.startTestMode()

		addr := types.Address{0x1}
		for nonce := uint64(0); nonce < 100; nonce++ {
			go func(nonce uint64) {
				err := pool.addTx(local, newDummyTx(addr, nonce, 1))
				assert.NoError(t, err)
			}(nonce)
		}

		waitUntilDone(done)

		assert.Equal(t, uint64(100), pool.gauge.read())
		assert.Equal(t, uint64(100), pool.accounts.from(addr).promoted.length())
	})
}

func TestAddTx1000(t *testing.T) {
	t.Run("send 1000 transactions from 10 accounts", func(t *testing.T) {
		accounts := []types.Address{
			addr1,
			addr2,
			addr3,
			addr4,
			addr5,
			addr6,
			addr7,
			addr8,
			addr9,
			addr10,
		}

		signer := crypto.NewEIP155Signer(uint64(100))
		key, _ := tests.GenerateKeyAndAddr(t)

		pool, err := newTestPool()
		assert.NoError(t, err)
		pool.AddSigner(signer)
		pool.EnableDev()

		// start the main loop
		done := pool.startTestMode()

		// send 1000
		for _, addr := range accounts {
			for nonce := uint64(0); nonce < 100; nonce++ {
				tx, err := signer.SignTx(newDummyTx(addr, nonce, 3), key)
				assert.NoError(t, err)
				go func(nonce uint64) {
					err := pool.addTx(local, tx)
					assert.NoError(t, err)
				}(nonce)
			}
		}

		waitUntilDone(done)

		assert.Equal(t, uint64(3000), pool.gauge.read())
		// assert.Equal(t, uint64(1000), pool.promoted.length())
	})
}

func TestResetAccounts(t *testing.T) {
	testCases := []struct {
		name      string
		allTxs    map[types.Address]transactions
		newNonces map[types.Address]uint64
		expected  result
	}{
		{
			name: "reset promoted only",
			allTxs: map[types.Address]transactions{ // all txs will end up in promoted queue
				addr1: {
					newDummyTx(addr1, 0, 1),
					newDummyTx(addr1, 1, 1),
					newDummyTx(addr1, 2, 1),
					newDummyTx(addr1, 3, 1),
				},
				addr2: {
					newDummyTx(addr2, 0, 1),
					newDummyTx(addr2, 1, 1),
				},
				addr3: {
					newDummyTx(addr3, 0, 1),
					newDummyTx(addr3, 1, 1),
					newDummyTx(addr3, 2, 1),
				},
				addr4: {
					newDummyTx(addr4, 0, 1),
					newDummyTx(addr4, 1, 1),
					newDummyTx(addr4, 2, 1),
					newDummyTx(addr4, 3, 1),
					newDummyTx(addr4, 4, 1),
				},
			},
			newNonces: map[types.Address]uint64{
				addr1: 2,
				addr2: 1,
				addr3: 0,
				addr4: 5,
			},
			expected: result{
				accounts: map[types.Address]accountState{
					addr1: {
						promoted: 2,
					},
					addr2: {
						promoted: 1,
					},
					addr3: {
						promoted: 3,
					},
					addr4: {
						promoted: 0,
					},
				},
				slots: 2 + 1 + 3 + 0,
			},
		},
		{
			name: "reset enqueued only",
			allTxs: map[types.Address]transactions{
				addr1: {
					newDummyTx(addr1, 3, 1),
					newDummyTx(addr1, 4, 1),
					newDummyTx(addr1, 5, 1),
				},
				addr2: {
					newDummyTx(addr2, 2, 1),
					newDummyTx(addr2, 3, 1),
					newDummyTx(addr2, 5, 1),
					newDummyTx(addr2, 6, 1),
					newDummyTx(addr2, 7, 1),
				},
				addr3: {
					newDummyTx(addr3, 7, 1),
					newDummyTx(addr3, 8, 1),
					newDummyTx(addr3, 9, 1),
				},
			},
			newNonces: map[types.Address]uint64{
				addr1: 3,
				addr2: 4,
				addr3: 8,
			},
			expected: result{
				accounts: map[types.Address]accountState{
					addr1: {
						enqueued: 0,
						promoted: 3, // reset will promote
					},
					addr2: {
						enqueued: 3,
						promoted: 0,
					},
					addr3: {
						enqueued: 0,
						promoted: 2, // reset will promote
					},
				},
				slots: 3 + 3 + 2,
			},
		},
		{
			name: "reset all queues",
			allTxs: map[types.Address]transactions{
				addr1: {
					// promoted
					newDummyTx(addr1, 0, 3),
					newDummyTx(addr1, 1, 3),
					newDummyTx(addr1, 2, 1),
					// enqueued
					newDummyTx(addr1, 5, 2),
					newDummyTx(addr1, 6, 2),
					newDummyTx(addr1, 8, 2),
				},
				addr2: {
					// promoted
					newDummyTx(addr2, 0, 2),
					newDummyTx(addr2, 1, 1),
					// enqueued
					newDummyTx(addr2, 4, 1),
					newDummyTx(addr2, 5, 2),
				},
				addr3: {
					// promoted
					newDummyTx(addr3, 0, 1),
					newDummyTx(addr3, 1, 2),
					newDummyTx(addr3, 2, 1),
					// enqueued
					newDummyTx(addr3, 4, 3),
					newDummyTx(addr3, 7, 1),
					newDummyTx(addr3, 9, 2),
				},
				addr4: {
					// promoted
					newDummyTx(addr4, 0, 1),
					newDummyTx(addr4, 1, 1),
					newDummyTx(addr4, 2, 2),
					newDummyTx(addr4, 3, 1),
				},
				addr5: {
					// enqueued
					newDummyTx(addr5, 6, 1),
					newDummyTx(addr5, 8, 1),
					newDummyTx(addr5, 9, 2),
					newDummyTx(addr5, 10, 1),
				},
			},
			newNonces: map[types.Address]uint64{
				addr1: 5,
				addr2: 3,
				addr3: 7,
				addr4: 2,
				addr5: 6,
			},
			expected: result{
				accounts: map[types.Address]accountState{
					addr1: {
						enqueued: 1,
						promoted: 2,
					},
					addr2: {
						enqueued: 2,
						promoted: 0,
					},
					addr3: {
						enqueued: 1,
						promoted: 1,
					},
					addr4: {
						enqueued: 0,
						promoted: 2,
					},
					addr5: {
						enqueued: 3,
						promoted: 1,
					},
				},
				slots: 6 + 3 + 3 + 3 + 5,
			},
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			pool, err := newTestPool()
			assert.NoError(t, err)
			pool.AddSigner(&mockSigner{})
			pool.EnableDev()

			// start the main loop
			done := pool.startTestMode()

			{ // setup prestate
				for _, txs := range test.allTxs {
					for _, tx := range txs {
						go func(tx *types.Transaction) {
							err := pool.addTx(local, tx)
							assert.NoError(t, err)
						}(tx)

					}
				}
				waitUntilDone(done)
			}

			pool.resetAccounts(test.newNonces)
			waitUntilDone(done)

			assert.Equal(t, test.expected.slots, pool.gauge.read())
			for addr := range test.expected.accounts {
				assert.Equal(t, // enqueued
					test.expected.accounts[addr].enqueued,
					pool.accounts.from(addr).enqueued.length())

				assert.Equal(t, // promoted
					test.expected.accounts[addr].promoted,
					pool.accounts.from(addr).promoted.length())
			}
		})
	}
}

func TestExecutablesOrder(t *testing.T) {
	newPricedTx := func(addr types.Address, nonce, gasPrice uint64) *types.Transaction {
		tx := newDummyTx(addr, nonce, 1)
		tx.GasPrice.SetUint64(gasPrice)

		return tx
	}

	testCases := []struct {
		name               string
		allTxs             map[types.Address]transactions
		expectedPriceOrder []uint64
	}{
		{
			name: "case #1",
			allTxs: map[types.Address]transactions{
				addr1: {
					newPricedTx(addr1, 0, 1),
				},
				addr2: {
					newPricedTx(addr2, 0, 2),
				},
				addr3: {
					newPricedTx(addr3, 0, 3),
				},
				addr4: {
					newPricedTx(addr4, 0, 4),
				},
				addr5: {
					newPricedTx(addr5, 0, 5),
				},
			},
			expectedPriceOrder: []uint64{
				5,
				4,
				3,
				2,
				1,
			},
		},
		{
			name: "case #2",
			allTxs: map[types.Address]transactions{
				addr1: {
					newPricedTx(addr1, 0, 3),
					newPricedTx(addr1, 1, 3),
					newPricedTx(addr1, 2, 3),
				},
				addr2: {
					newPricedTx(addr2, 0, 2),
					newPricedTx(addr2, 1, 2),
					newPricedTx(addr2, 2, 2),
				},
				addr3: {
					newPricedTx(addr3, 0, 1),
					newPricedTx(addr3, 1, 1),
					newPricedTx(addr3, 2, 1),
				},
			},
			expectedPriceOrder: []uint64{
				3,
				3,
				3,
				2,
				2,
				2,
				1,
				1,
				1,
			},
		},
		{
			name: "case #3",
			allTxs: map[types.Address]transactions{
				addr1: {
					newPricedTx(addr1, 0, 9),
					newPricedTx(addr1, 1, 5),
					newPricedTx(addr1, 2, 3),
				},
				addr2: {
					newPricedTx(addr2, 0, 9),
					newPricedTx(addr2, 1, 3),
					newPricedTx(addr2, 2, 1),
				},
			},
			expectedPriceOrder: []uint64{
				9,
				9,
				5,
				3,
				3,
				1,
			},
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			pool, err := newTestPool()
			assert.NoError(t, err)
			pool.AddSigner(&mockSigner{})
			pool.EnableDev()

			// start the main loop
			done := pool.startTestMode()

			for _, txs := range test.allTxs {
				for _, tx := range txs {
					// send all txs
					go func(tx *types.Transaction) {
						err := pool.addTx(local, tx)
						assert.NoError(t, err)
					}(tx)
				}
			}
			waitUntilDone(done)
			assert.Equal(t, uint64(len(test.expectedPriceOrder)), pool.accounts.promoted())

			var successful transactions
			for {
				tx := pool.Peek()
				if tx == nil {
					break
				}

				pool.Pop(tx)
				successful = append(successful, tx)
			}

			// verify the highest priced transactions
			// were processed first
			for i, tx := range successful {
				assert.Equal(t, test.expectedPriceOrder[i], tx.GasPrice.Uint64())
			}
		})
	}
}

type status int

// Status of a transaction resulted
// from a transition write attempt
const (
	recoverable status = iota

	/* if a tx is unrecoverable,
	all subsequent ones are
	considered recoverable (nonce mismatch)
	*/
	unrecoverable

	ok
)

type statusTx struct {
	tx     *types.Transaction
	status status
}

func TestRecovery(t *testing.T) {
	testCases := []struct {
		name     string
		allTxs   map[types.Address][]statusTx
		expected result
	}{
		{
			name: "all recovered in enqueued",
			allTxs: map[types.Address][]statusTx{
				addr1: {
					{newDummyTx(addr1, 0, 1), ok},
					{newDummyTx(addr1, 1, 1), unrecoverable}, // will demote subsequent
					{newDummyTx(addr1, 2, 1), recoverable},
					{newDummyTx(addr1, 3, 1), recoverable},
					{newDummyTx(addr1, 4, 1), recoverable},
				},
				addr2: {
					{newDummyTx(addr2, 0, 1), unrecoverable}, // will demote subsequent
					{newDummyTx(addr2, 1, 1), recoverable},
				},
				addr3: {
					{newDummyTx(addr3, 0, 1), ok},
					{newDummyTx(addr3, 1, 1), unrecoverable}, // will demote subsequent
					{newDummyTx(addr3, 2, 1), recoverable},
					{newDummyTx(addr3, 3, 1), recoverable},
				},
			},
			expected: result{
				slots: 3 + 1 + 2,
				accounts: map[types.Address]accountState{
					addr1: {
						enqueued: 3,
					},
					addr2: {
						enqueued: 1,
					},
					addr3: {
						enqueued: 2,
					},
				},
			},
		},
		{
			name: "all recovered in promoted",
			allTxs: map[types.Address][]statusTx{
				addr1: {
					{newDummyTx(addr1, 0, 1), ok},
					{newDummyTx(addr1, 1, 1), ok},
					{newDummyTx(addr1, 2, 1), recoverable},
					{newDummyTx(addr1, 3, 1), recoverable},
					{newDummyTx(addr1, 4, 1), recoverable},
				},
				addr2: {
					{newDummyTx(addr2, 0, 1), ok},
					{newDummyTx(addr2, 1, 1), ok},
				},
				addr3: {
					{newDummyTx(addr3, 0, 1), ok},
					{newDummyTx(addr3, 1, 1), ok},
					{newDummyTx(addr3, 2, 1), ok},
					{newDummyTx(addr3, 3, 1), recoverable},
				},
				addr4: {
					{newDummyTx(addr4, 0, 1), ok},
					{newDummyTx(addr4, 1, 1), recoverable},
					{newDummyTx(addr4, 2, 1), recoverable},
				},
			},
			expected: result{
				slots: 3 + 0 + 1 + 2,
				accounts: map[types.Address]accountState{
					addr1: {
						promoted: 3,
					},
					addr2: {
						promoted: 0,
					},
					addr3: {
						promoted: 1,
					},
					addr4: {
						promoted: 2,
					},
				},
			},
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			// helper callback for transition errors
			status := func(tx *types.Transaction) (s status) {
				txs := test.allTxs[tx.From]
				for _, sTx := range txs {
					if tx.Nonce == sTx.tx.Nonce {
						s = sTx.status
					}
				}

				return
			}

			// create pool
			pool, err := newTestPool()
			assert.NoError(t, err)
			pool.AddSigner(&mockSigner{})
			pool.EnableDev()

			done := pool.startTestMode()

			{ // setup prestate
				for _, txs := range test.allTxs {
					for _, sTx := range txs {
						go func(tx *types.Transaction) {
							err := pool.addTx(local, tx)
							assert.NoError(t, err)
						}(sTx.tx)
					}
				}
				waitUntilDone(done)
			}

			// mock ibft.writeTransactions()
			func() {
				pool.Prepare()
				for {
					tx := pool.Peek()
					if tx == nil {
						break
					}

					switch status(tx) {
					case recoverable:
						pool.Demote(tx)
					case unrecoverable:
						pool.Drop(tx)
					case ok:
						pool.Pop(tx)
					}
				}
			}()

			// pool was handling requests
			waitUntilDone(done)

			assert.Equal(t, test.expected.slots, pool.gauge.read())
			for addr := range test.expected.accounts {
				assert.Equal(t, // enqueued
					test.expected.accounts[addr].enqueued,
					pool.accounts.from(addr).enqueued.length())

				assert.Equal(t, // promoted
					test.expected.accounts[addr].promoted,
					pool.accounts.from(addr).promoted.length())
			}
		})
	}
}
