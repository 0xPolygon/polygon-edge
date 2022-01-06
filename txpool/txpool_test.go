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
	validGasLimit     uint64 = 100000
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
	rand.Read(input)
	return &types.Transaction{
		From:     addr,
		Nonce:    nonce,
		Value:    big.NewInt(1),
		GasPrice: big.NewInt(0).SetUint64(defaultPriceLimit),
		Gas:      100000000,
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

type result struct {
	enqueued map[types.Address]uint64
	promoted uint64
	slots    uint64
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
				From:     types.Address{0x1},
				Value:    big.NewInt(1),
				GasPrice: big.NewInt(1),
				Gas:      validGasLimit,
			},
		},
		{
			name:          "ErrTxPoolOverflow",
			expectedError: ErrTxPoolOverflow,
			tx: &types.Transaction{
				From:     types.Address{0x1},
				Value:    big.NewInt(1),
				GasPrice: big.NewInt(1),
				Gas:      validGasLimit,
			},
		},
		{
			name:          "ErrIntrinsicGas",
			expectedError: ErrIntrinsicGas,
			tx: &types.Transaction{
				From:     types.Address{0x1},
				Value:    big.NewInt(1),
				GasPrice: big.NewInt(1),
				Gas:      1,
			},
		},
		{
			name:          "ErrAlreadyKnown",
			expectedError: ErrAlreadyKnown,
			tx: &types.Transaction{
				From:     types.Address{0x1},
				Value:    big.NewInt(1),
				GasPrice: big.NewInt(1),
				Gas:      validGasLimit,
			},
		},
		{
			name:          "ErrOversizedData",
			expectedError: ErrOversizedData,
			tx: &types.Transaction{
				From:     types.Address{0x1},
				Value:    big.NewInt(1),
				GasPrice: big.NewInt(1),
				Gas:      validGasLimit,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pool, err := newTestPool()
			assert.NoError(t, err)
			pool.EnableDev()
			pool.AddSigner(crypto.NewEIP155Signer(100))

			/* special error setups */
			switch tc.expectedError {
			case ErrTxPoolOverflow:
				pool.gauge.increase(defaultMaxSlots)
			case ErrNonEncryptedTx:
				pool.dev = false
			case ErrAlreadyKnown:
				go pool.addTx(local, tc.tx)
				go pool.handleAddRequest(<-pool.addReqCh)
				<-pool.promoteReqCh
			case ErrInvalidAccountState:
				pool.store = faultyMockStore{}
			case ErrOversizedData:
				data := make([]byte, 989898)
				rand.Read(data)
				tc.tx.Input = data
			}

			assert.ErrorIs(t,
				tc.expectedError,
				pool.addTx(local, tc.tx),
			)
		})
	}
}

func TestAddHandler(t *testing.T) {
	t.Run("enqueue higher nonce txs", func(t *testing.T) {
		pool, err := newTestPool()
		assert.NoError(t, err)
		pool.AddSigner(&mockSigner{})
		pool.EnableDev()

		addr := types.Address{0x1}

		for i := uint64(10); i < 20; i++ {
			go pool.addTx(local, newDummyTx(addr, i, 1))
			pool.handleAddRequest(<-pool.addReqCh)
		}

		assert.Equal(t, uint64(10), pool.accounts.from(addr).length())
	})

	t.Run("reject low nonce txs", func(t *testing.T) {
		pool, err := newTestPool()
		assert.NoError(t, err)
		pool.AddSigner(&mockSigner{})
		pool.EnableDev()

		addr := types.Address{0x1}

		{ // set up prestate
			pool.createAccountOnce(addr)
			assert.Equal(t, uint64(0), pool.accounts.from(addr).length())

			pool.nextNonces.store(addr, 20)
			nextNonce, ok := pool.nextNonces.load(addr)
			assert.True(t, ok)
			assert.Equal(t, uint64(20), nextNonce)
		}

		// send txs
		for i := uint64(0); i < 10; i++ {
			go pool.addTx(local, newDummyTx(addr, i, 1))
			pool.handleAddRequest(<-pool.addReqCh)
		}

		assert.Equal(t, uint64(0), pool.accounts.from(addr).length())
	})

	t.Run("signal promotion", func(t *testing.T) {
		pool, err := newTestPool()
		assert.NoError(t, err)
		pool.AddSigner(&mockSigner{})
		pool.EnableDev()

		addr := types.Address{0x1}

		// send tx
		go pool.addTx(local, newDummyTx(addr, 0, 1)) // fresh account
		go pool.handleAddRequest(<-pool.addReqCh)
		req := <-pool.promoteReqCh

		assert.Equal(t, uint64(1), pool.accounts.from(addr).length())
		assert.Equal(t, addr, req.account)
		assert.Equal(t, uint64(0), pool.promoted.length()) // promotions are done in handlePromoteReqest()
	})

	t.Run("enqueue returnee with high nonce (rollback)", func(t *testing.T) {
		pool, err := newTestPool()
		assert.NoError(t, err)
		pool.AddSigner(&mockSigner{})
		pool.EnableDev()

		addr := types.Address{0x1}

		{ // set up prestate
			pool.createAccountOnce(addr)
			assert.Equal(t, uint64(0), pool.accounts.from(addr).length())

			pool.nextNonces.store(addr, 5)
			nextNonce, ok := pool.nextNonces.load(addr)
			assert.True(t, ok)
			assert.Equal(t, uint64(5), nextNonce)
		}

		// send returnee (a previously promoted tx that is being recovered)
		go pool.handleAddRequest(addRequest{
			tx:      newDummyTx(addr, 3, 1),
			demoted: true,
		})

		req := <-pool.promoteReqCh
		assert.Equal(t, addr, req.account)
		assert.Equal(t, uint64(1), pool.accounts.from(addr).length())

		tx := pool.accounts.from(addr).queue.Peek()
		assert.Equal(t, uint64(3), tx.Nonce)
	})
}

func TestPromoteHandler(t *testing.T) {
	t.Run("one transaction one promotion", func(t *testing.T) {
		pool, err := newTestPool()
		assert.NoError(t, err)
		pool.AddSigner(&mockSigner{})
		pool.EnableDev()

		from := types.Address{0x1}
		dummyTx := &types.Transaction{
			From:     from,
			Nonce:    0,
			Value:    big.NewInt(1),
			GasPrice: big.NewInt(1),
			Gas:      validGasLimit,
		}

		// 1. add
		go pool.addTx(local, dummyTx)
		go pool.handleAddRequest(<-pool.addReqCh)
		promReq := <-pool.promoteReqCh

		assert.Equal(t, uint64(1), pool.accounts.from(addr1).length())

		// 2. promote
		pool.handlePromoteRequest(promReq)

		assert.Equal(t, uint64(0), pool.accounts.from(addr1).length())
		assert.Equal(t, uint64(1), pool.promoted.length())
	})

	t.Run("first transaction comes in last", func(t *testing.T) {
		pool, err := newTestPool()
		assert.NoError(t, err)
		pool.AddSigner(&mockSigner{})
		pool.EnableDev()

		var addRequests []addRequest
		for i := uint64(0); i < 10; i++ {
			go pool.addTx(local, newDummyTx(addr1, i, 1))
			addRequests = append(addRequests, <-pool.addReqCh)
		}

		// receive in the reverse order
		for i := uint64(9); i > 0; i-- {
			pool.handleAddRequest(addRequests[i])
		}

		assert.Equal(t, uint64(9), pool.accounts.from(addr1).length())

		go pool.handleAddRequest(addRequests[0])
		pool.handlePromoteRequest(<-pool.promoteReqCh)

		assert.Equal(t, uint64(0), pool.accounts.from(addr1).length())
		assert.Equal(t, uint64(10), pool.promoted.length())
	})

	t.Run("six transactions two promotions", func(t *testing.T) {
		pool, err := newTestPool()
		assert.NoError(t, err)
		pool.AddSigner(&mockSigner{})
		pool.EnableDev()

		// send first 3
		var addRequests []addRequest
		for i := uint64(0); i < 3; i++ {
			go pool.addTx(local, newDummyTx(addr1, i, 1))
			addRequests = append(addRequests, <-pool.addReqCh)
		}

		// 1st tx triggers 1st promotion
		go pool.handleAddRequest(addRequests[0])
		promReq := <-pool.promoteReqCh
		assert.Equal(t, uint64(1), pool.accounts.from(addr1).length())

		// enqueue 2nd and 3rd txs
		pool.handleAddRequest(addRequests[1])
		pool.handleAddRequest(addRequests[2])
		assert.Equal(t, uint64(3), pool.accounts.from(addr1).length())

		// // 1st promotion occurs
		pool.handlePromoteRequest(promReq)
		assert.Equal(t, uint64(0), pool.accounts.from(addr1).length())
		assert.Equal(t, uint64(3), pool.promoted.length())

		// send last 3
		for i := uint64(3); i < 6; i++ {
			go pool.addTx(local, newDummyTx(addr1, i, 1))
			addRequests = append(addRequests, <-pool.addReqCh)
		}

		// 4th tx triggers 2nd promotion
		go pool.handleAddRequest(addRequests[3])
		promReq = <-pool.promoteReqCh

		assert.Equal(t, uint64(1), pool.accounts.from(addr1).length())

		// enqueue last 2
		pool.handleAddRequest(addRequests[4])
		pool.handleAddRequest(addRequests[5])

		assert.Equal(t, uint64(3), pool.accounts.from(addr1).length())

		// 2nd promotion occurs
		pool.handlePromoteRequest(promReq)
		assert.Equal(t, uint64(0), pool.accounts.from(addr1).length())
		assert.Equal(t, uint64(6), pool.promoted.length())
	})

	t.Run("promote returnee with low nonce (recovery)", func(t *testing.T) {
		pool, err := newTestPool()
		assert.NoError(t, err)
		pool.AddSigner(&mockSigner{})
		pool.EnableDev()

		addr := types.Address{0x1}

		{ // setup prestate
			pool.createAccountOnce(addr)
			assert.Equal(t, uint64(0), pool.promoted.length())
			pool.nextNonces.store(addr, 7)

			nextNonce, ok := pool.nextNonces.load(addr)
			assert.True(t, ok)
			assert.Equal(t, uint64(7), nextNonce)
		}

		// send recovered tx
		go pool.handleAddRequest(addRequest{
			tx:      newDummyTx(addr, 4, 1),
			demoted: true,
		})

		// promote recovered
		pool.handlePromoteRequest(<-pool.promoteReqCh)
		assert.Equal(t, uint64(1), pool.promoted.length())

		// verify the returnee is in promoted queue
		tx := pool.promoted.peek()
		assert.Equal(t, uint64(4), tx.Nonce)

		// next nonce isn't updated for returnees
		nextNonce, _ := pool.nextNonces.load(addr)
		assert.Equal(t, uint64(7), nextNonce)
	})
}

// account queue (enqueued)
func TestResetHandlerEnqueued(t *testing.T) {
	t.Run("reset causes promotion", func(t *testing.T) {
		testCases := []struct {
			name     string
			enqueued []uint64 // nonces
			newNonce uint64
			expected result
		}{
			{
				name:     "prune some enqueued transactions",
				enqueued: []uint64{3, 4, 5},
				newNonce: 4,
				expected: result{
					enqueued: map[types.Address]uint64{
						addr1: 0,
					},
					promoted: 2,
				},
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				pool, err := newTestPool()
				assert.NoError(t, err)
				pool.AddSigner(&mockSigner{})
				pool.EnableDev()

				{ // setup prestate
					for _, nonce := range tc.enqueued {
						go pool.addTx(local, newDummyTx(addr1, nonce, 1))
						pool.handleAddRequest(<-pool.addReqCh)
					}
					assert.Equal(t, uint64(len(tc.enqueued)), pool.accounts.from(addr1).length())
				}

				// align account queue with reset event
				go pool.resetQueues(map[types.Address]uint64{
					addr1: tc.newNonce,
				})

				// this will signal a promotion for addr1
				req := <-pool.promoteReqCh
				pool.handlePromoteRequest(req)

				assert.Equal(t, tc.expected.enqueued[addr1], pool.accounts.from(addr1).length())
				assert.Equal(t, tc.expected.promoted, pool.promoted.length())
			})
		}

	})

	t.Run("reset causes no promotion", func(t *testing.T) {
		testCases := []struct {
			name     string
			enqueued []uint64 // nonces
			newNonce uint64
			expected result
		}{
			{
				name:     "prune all enqueued transactions",
				enqueued: []uint64{2, 5, 6, 8},
				newNonce: 10,
				expected: result{
					enqueued: map[types.Address]uint64{
						addr1: 0,
					},
					promoted: 0,
				},
			},
			{
				name:     "no enqueued transactions to prune",
				enqueued: []uint64{9, 10, 12},
				newNonce: 7,
				expected: result{
					enqueued: map[types.Address]uint64{
						addr1: 3,
					},
					promoted: 0,
				},
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				pool, err := newTestPool()
				assert.NoError(t, err)
				pool.AddSigner(&mockSigner{})
				pool.EnableDev()

				{ // setup prestate
					for _, nonce := range tc.enqueued {
						go pool.addTx(local, newDummyTx(addr1, nonce, 1))
						pool.handleAddRequest(<-pool.addReqCh)
					}
					assert.Equal(t, uint64(len(tc.enqueued)), pool.accounts.from(addr1).length())
				}

				pool.resetQueues(map[types.Address]uint64{
					addr1: tc.newNonce,
				})

				assert.Equal(t, tc.expected.enqueued[addr1], pool.accounts.from(addr1).length())
				assert.Equal(t, tc.expected.promoted, pool.promoted.length())
			})
		}

	})
}

// promoted
func TestResetHandlerPromoted(t *testing.T) {
	testCases := []struct {
		name             string
		promoted         []uint64 // nonces
		newNonce         uint64
		expectedPromoted uint64
	}{
		{
			name:             "prune some promoted transactions",
			promoted:         []uint64{0, 1, 2, 3, 4},
			newNonce:         1,
			expectedPromoted: 4,
		},
		{
			name:             "prune all promoted transactions",
			promoted:         []uint64{5, 6, 7},
			newNonce:         9,
			expectedPromoted: 0,
		},
		{
			name:             "no promoted transactions to prune",
			promoted:         []uint64{5, 6},
			newNonce:         4,
			expectedPromoted: 2,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pool, err := newTestPool()
			assert.NoError(t, err)
			pool.AddSigner(&mockSigner{})
			pool.EnableDev()

			addr := types.Address{0x1}

			{ // setup prestate
				pool.createAccountOnce(addr)
				pool.nextNonces.store(addr, tc.promoted[0])
				initialPromoted := uint64(len(tc.promoted))

				// save the first promotion for later
				go pool.addTx(local, newDummyTx(addr, tc.promoted[0], 1))
				go pool.handleAddRequest(<-pool.addReqCh)
				prom := <-pool.promoteReqCh

				// enqueue remaining txs
				tc.promoted = tc.promoted[1:]
				for _, nonce := range tc.promoted {
					go pool.addTx(local, newDummyTx(addr, nonce, 1))
					pool.handleAddRequest(<-pool.addReqCh)
				}

				// run the first promotion
				pool.handlePromoteRequest(prom)
				assert.Equal(t, initialPromoted, pool.promoted.length())
			}

			// align promoted queue with reset event
			pool.resetQueues(map[types.Address]uint64{
				addr1: tc.newNonce,
			})

			assert.Equal(t, tc.expectedPromoted, pool.promoted.length())
		})
	}
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
		for i := uint64(0); i < 100; i++ {
			go pool.addTx(local, newDummyTx(addr, i, 1))
		}

		waitUntilDone(done)

		assert.Equal(t, uint64(100), pool.gauge.read())
		assert.Equal(t, uint64(100), pool.promoted.length())
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
			for i := uint64(0); i < 100; i++ {
				tx, err := signer.SignTx(newDummyTx(addr, i, 3), key)
				assert.NoError(t, err)
				go pool.addTx(local, tx)
			}
		}

		waitUntilDone(done)

		assert.Equal(t, uint64(3000), pool.gauge.read())
		assert.Equal(t, uint64(1000), pool.promoted.length())
	})
}

func TestResetWithHeader(t *testing.T) {
	testCases := []struct {
		name      string
		all       map[types.Address]transactions
		newNonces map[types.Address]uint64
		expected  result
	}{
		{
			name: "reset promoted only",
			all: map[types.Address]transactions{ // all txs will end up in promoted queue
				addr1: {
					newDummyTx(addr1, 0, 1),
					newDummyTx(addr1, 1, 1),
					newDummyTx(addr1, 2, 1),
					newDummyTx(addr1, 3, 1),
					newDummyTx(addr1, 4, 1),
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
			},
			newNonces: map[types.Address]uint64{
				addr1: 2,
				addr2: 1,
				addr3: 0,
				addr4: 5,
			},
			expected: result{
				enqueued: map[types.Address]uint64{
					addr1: 0,
					addr2: 0,
					addr3: 0,
				},
				promoted: 7,
				slots:    3 + 1 + 3,
			},
		},
		{
			name: "reset enqueued only",
			all: map[types.Address]transactions{
				addr1: {
					newDummyTx(addr1, 3, 1),
					newDummyTx(addr1, 4, 1),
					newDummyTx(addr1, 5, 1),
				},
				addr2: {
					newDummyTx(addr2, 1, 1),
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
				addr2: 5,
				addr3: 8,
			},
			expected: result{
				enqueued: map[types.Address]uint64{
					addr1: 0,
					addr2: 0,
					addr3: 0,
				},
				promoted: 3 + 3 + 2,
				slots:    3 + 3 + 2,
			},
		},
		{
			name: "reset all queues",
			all: map[types.Address]transactions{
				addr1: {
					newDummyTx(addr1, 3, 3), // dropped
					newDummyTx(addr1, 4, 3), // dropped
					newDummyTx(addr1, 5, 1),
					newDummyTx(addr1, 6, 2),
				},
				addr2: {
					newDummyTx(addr2, 1, 2), // dropped
					newDummyTx(addr2, 5, 1),
					newDummyTx(addr2, 6, 1),
					newDummyTx(addr2, 7, 2),
				},
				addr3: {
					newDummyTx(addr3, 0, 1), // dropped
					newDummyTx(addr3, 1, 2), // dropped
					newDummyTx(addr3, 2, 1), // dropped
					newDummyTx(addr3, 4, 3), // dropped
					newDummyTx(addr3, 7, 1),
					newDummyTx(addr3, 9, 2),
				},
				addr4: {
					newDummyTx(addr4, 0, 1), // dropped
					newDummyTx(addr4, 1, 1), // dropped
					newDummyTx(addr4, 2, 2),
					newDummyTx(addr4, 3, 1),
				},
			},
			newNonces: map[types.Address]uint64{
				addr1: 5,
				addr2: 3,
				addr3: 7,
				addr4: 2,
			},
			expected: result{
				promoted: 2 + 0 + 1 + 2,
				enqueued: map[types.Address]uint64{
					addr1: 0,
					addr2: 3,
					addr3: 1,
					addr4: 0,
				},
				slots: 3 + 4 + 3 + 3,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pool, err := newTestPool()
			assert.NoError(t, err)
			pool.AddSigner(&mockSigner{})
			pool.EnableDev()

			// start the main loop
			done := pool.startTestMode()

			{ // setup initial state
				for _, txs := range tc.all {
					for _, tx := range txs {
						go pool.addTx(local, tx)
					}
				}
				waitUntilDone(done)
			}

			// ResetWitHeader would invoke this handler when a node
			// is building/discovering a new block
			pool.resetQueues(tc.newNonces)
			waitUntilDone(done)

			for addr, count := range tc.expected.enqueued {
				assert.Equal(t, count, pool.accounts.from(addr).length())
			}
			assert.Equal(t, tc.expected.promoted, pool.promoted.length())
			assert.Equal(t, tc.expected.slots, pool.gauge.read())
		})
	}
}

type status int

const (
	// depending on the value of next nonce
	// recoverables (returnees) can end up
	// either in enqueued or in promoted
	recoverable status = iota

	// rollback nonce for this account (see recoverable)
	unrecoverable

	// all good, no need to affect the pool in any way
	ok
)

type statusTx struct {
	tx     *types.Transaction
	status status
}

func TestRecoverySingleAccount(t *testing.T) {
	testCases := []struct {
		name         string
		transactions []statusTx
		expected     result
	}{
		{ /*
				If tx execution fails for some account,
				subsequent transctions will be regarded as recoverable
				due to higher nonce, ending up back in enqueued
			*/
			name: "all recovered in enqueued",
			transactions: []statusTx{
				{newDummyTx(addr1, 0, 1), unrecoverable},
				{newDummyTx(addr1, 1, 1), recoverable},
				{newDummyTx(addr1, 2, 1), recoverable},
				{newDummyTx(addr1, 3, 1), recoverable},
				{newDummyTx(addr1, 4, 1), recoverable},
				{newDummyTx(addr1, 5, 1), recoverable},
			},
			expected: result{
				enqueued: map[types.Address]uint64{
					addr1: 5,
				},
				promoted: 0,
				slots:    5,
			},
		},
		{
			name: "all recovered in promoted",
			transactions: []statusTx{
				{newDummyTx(addr1, 0, 1), ok},
				{newDummyTx(addr1, 1, 1), recoverable},
				{newDummyTx(addr1, 2, 1), recoverable},
				{newDummyTx(addr1, 3, 1), recoverable},
				{newDummyTx(addr1, 4, 1), recoverable},
				{newDummyTx(addr1, 5, 1), recoverable},
			},
			expected: result{
				enqueued: map[types.Address]uint64{
					addr1: 0,
				},
				promoted: 5,
				slots:    5,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// helper callback for transition errors
			status := func(tx *types.Transaction) status {
				var status status
				for _, sTx := range tc.transactions {
					if tx.Nonce == sTx.tx.Nonce {
						status = sTx.status
					}
				}

				return status
			}

			// create pool
			pool, err := newTestPool()
			assert.NoError(t, err)
			pool.AddSigner(&mockSigner{})
			pool.EnableDev()

			done := pool.startTestMode()

			// setup initial state
			{
				for _, sTx := range tc.transactions {
					go pool.addTx(local, sTx.tx)
				}
				waitUntilDone(done)
			}

			// mock ibft.writeTransactions()
			func() {
				pool.LockPromoted(true)
				for {
					tx := pool.Peek()
					if tx == nil {
						break
					}

					switch status(tx) {
					case recoverable:
						pool.Demote()
					case unrecoverable:
						pool.Drop()
					case ok:
						pool.Pop()
					}
				}
				pool.UnlockPromoted()
			}()

			// pool was handling requests
			waitUntilDone(done)

			// // assert
			assert.Equal(t, tc.expected.slots, pool.gauge.read())
			assert.Equal(t, tc.expected.promoted, pool.promoted.length())
			assert.Equal(t, tc.expected.enqueued[addr1], pool.accounts.from(addr1).length())
		})
	}

}

func TestRecoveryMultipleAccounts(t *testing.T) {
	testCases := []struct {
		name         string
		transactions map[types.Address][]statusTx
		expected     result
	}{
		{
			name: "all recovered in enqueued",
			transactions: map[types.Address][]statusTx{
				addr1: {
					{newDummyTx(addr1, 0, 1), ok},
				},
				addr2: {
					{newDummyTx(addr2, 0, 1), ok},
					{newDummyTx(addr2, 1, 1), unrecoverable},
					{newDummyTx(addr2, 2, 1), recoverable},
					{newDummyTx(addr2, 3, 1), recoverable},
				},
				addr3: {
					{newDummyTx(addr3, 0, 1), unrecoverable},
					{newDummyTx(addr3, 1, 1), recoverable},
				},
				addr4: {
					{newDummyTx(addr4, 0, 1), ok},
					{newDummyTx(addr4, 1, 1), ok},
					{newDummyTx(addr4, 2, 1), unrecoverable},
				},
			},
			expected: result{
				enqueued: map[types.Address]uint64{
					addr1: 0,
					addr2: 2,
					addr3: 1,
					addr4: 0,
				},
				promoted: 0,
				slots:    3,
			},
		},
		{
			name: "all recovered in promoted",
			transactions: map[types.Address][]statusTx{
				addr1: {
					{newDummyTx(addr1, 0, 1), ok},
					{newDummyTx(addr1, 1, 1), recoverable},
					{newDummyTx(addr1, 2, 1), recoverable},
				},
				addr2: {
					{newDummyTx(addr2, 0, 1), ok},
				},
				addr3: {
					{newDummyTx(addr3, 0, 1), recoverable},
					{newDummyTx(addr3, 1, 1), recoverable},
				},
			},
			expected: result{
				enqueued: map[types.Address]uint64{
					addr1: 0,
					addr2: 0,
					addr3: 0,
					addr4: 0,
				},
				promoted: 4,
				slots:    4,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// helper callback for transition errors
			status := func(tx *types.Transaction) status {
				txs := tc.transactions[tx.From]

				var status status
				for _, sTx := range txs {
					if tx.Nonce == sTx.tx.Nonce {
						status = sTx.status
					}
				}

				return status
			}

			// create pool
			pool, err := newTestPool()
			assert.NoError(t, err)
			pool.AddSigner(&mockSigner{})
			pool.EnableDev()

			done := pool.startTestMode()

			// setup initial state
			{
				for _, txs := range tc.transactions {
					for _, sTx := range txs {
						go pool.addTx(local, sTx.tx)
					}
				}
				waitUntilDone(done)
			}

			// mock ibft.writeTransactions()
			func() {
				pool.LockPromoted(true)
				for {
					tx := pool.Peek()
					if tx == nil {
						break
					}

					switch status(tx) {
					case recoverable:
						pool.Demote()
					case unrecoverable:
						pool.Drop()
					case ok:
						pool.Pop()
					}
				}
				pool.UnlockPromoted()
			}()

			// pool was handling requests
			waitUntilDone(done)

			// // assert
			assert.Equal(t, tc.expected.slots, pool.gauge.read())
			assert.Equal(t, tc.expected.promoted, pool.promoted.length())
			for addr := range tc.transactions {
				assert.Equal(t, tc.expected.enqueued[addr], pool.accounts.from(addr).length())
			}
		})
	}
}
