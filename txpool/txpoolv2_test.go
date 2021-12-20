package txpool

import (
	"crypto/rand"
	"fmt"
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
		GasPrice: big.NewInt(1),
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
				go pool.addTx(tc.tx)
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
				pool.addTx(tc.tx),
			)
		})
	}
}

/* handleAddRequest tests */

func TestAddHandler(t *testing.T) {
	t.Run("enqueue higher nonce txs", func(t *testing.T) {
		pool, err := newTestPool()
		assert.NoError(t, err)
		pool.AddSigner(&mockSigner{})
		pool.EnableDev()

		addr := types.Address{0x1}

		for i := uint64(10); i < 20; i++ {
			go pool.addTx(newDummyTx(addr, i, 1))
			pool.handleAddRequest(<-pool.addReqCh)
		}

		assert.Equal(t, uint64(10), pool.enqueued.from(addr).length())
	})

	t.Run("reject low nonce txs", func(t *testing.T) {
		pool, err := newTestPool()
		assert.NoError(t, err)
		pool.AddSigner(&mockSigner{})
		pool.EnableDev()

		addr := types.Address{0x1}

		{ // set up prestate
			pool.createAccountOnce(addr)
			assert.Equal(t, uint64(0), pool.enqueued.from(addr).length())

			pool.nextNonces.store(addr, 20)
			nextNonce, ok := pool.nextNonces.load(addr)
			assert.True(t, ok)
			assert.Equal(t, uint64(20), nextNonce)
		}

		// send txs
		for i := uint64(0); i < 10; i++ {
			go pool.addTx(newDummyTx(addr, i, 1))
			pool.handleAddRequest(<-pool.addReqCh)
		}

		assert.Equal(t, uint64(0), pool.enqueued.from(addr).length())
	})

	t.Run("signal promotion", func(t *testing.T) {
		pool, err := newTestPool()
		assert.NoError(t, err)
		pool.AddSigner(&mockSigner{})
		pool.EnableDev()

		addr := types.Address{0x1}

		// send tx
		go pool.addTx(newDummyTx(addr, 0, 1)) // fresh account
		go pool.handleAddRequest(<-pool.addReqCh)
		req := <-pool.promoteReqCh

		assert.Equal(t, uint64(1), pool.enqueued.from(addr).length())
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
			assert.Equal(t, uint64(0), pool.enqueued.from(addr).length())

			pool.nextNonces.store(addr, 5)
			nextNonce, ok := pool.nextNonces.load(addr)
			assert.True(t, ok)
			assert.Equal(t, uint64(5), nextNonce)
		}

		// send returnee (a previously promoted tx that is being recovered)
		go pool.handleAddRequest(addRequest{
			tx:       newDummyTx(addr, 3, 1),
			returnee: true,
		})

		req := <-pool.promoteReqCh
		assert.Equal(t, addr, req.account)
		assert.Equal(t, uint64(1), pool.enqueued.from(addr).length())

		tx := pool.enqueued.from(addr).peek()
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
		go pool.addTx(dummyTx)
		go pool.handleAddRequest(<-pool.addReqCh)
		promReq := <-pool.promoteReqCh

		assert.Equal(t, uint64(1), pool.enqueued.from(addr1).length())

		// 2. promote
		pool.handlePromoteRequest(promReq)

		assert.Equal(t, uint64(0), pool.enqueued.from(addr1).length())
		assert.Equal(t, uint64(1), pool.promoted.length())
	})

	t.Run("first transaction comes in last", func(t *testing.T) {
		pool, err := newTestPool()
		assert.NoError(t, err)
		pool.AddSigner(&mockSigner{})
		pool.EnableDev()

		var addRequests []addRequest
		for i := uint64(0); i < 10; i++ {
			go pool.addTx(newDummyTx(addr1, i, 1))
			addRequests = append(addRequests, <-pool.addReqCh)
		}

		// receive in the reverse order
		for i := uint64(9); i > 0; i-- {
			pool.handleAddRequest(addRequests[i])
		}

		assert.Equal(t, uint64(9), pool.enqueued.from(addr1).length())

		go pool.handleAddRequest(addRequests[0])
		pool.handlePromoteRequest(<-pool.promoteReqCh)

		assert.Equal(t, uint64(0), pool.enqueued.from(addr1).length())
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
			go pool.addTx(newDummyTx(addr1, i, 1))
			addRequests = append(addRequests, <-pool.addReqCh)
		}

		// 1st tx triggers 1st promotion
		go pool.handleAddRequest(addRequests[0])
		promReq := <-pool.promoteReqCh
		assert.Equal(t, uint64(1), pool.enqueued.from(addr1).length())

		// enqueue 2nd and 3rd txs
		pool.handleAddRequest(addRequests[1])
		pool.handleAddRequest(addRequests[2])
		assert.Equal(t, uint64(3), pool.enqueued.from(addr1).length())

		// // 1st promotion occurs
		pool.handlePromoteRequest(promReq)
		assert.Equal(t, uint64(0), pool.enqueued.from(addr1).length())
		assert.Equal(t, uint64(3), pool.promoted.length())

		// send last 3
		for i := uint64(3); i < 6; i++ {
			go pool.addTx(newDummyTx(addr1, i, 1))
			addRequests = append(addRequests, <-pool.addReqCh)
		}

		// 4th tx triggers 2nd promotion
		go pool.handleAddRequest(addRequests[3])
		promReq = <-pool.promoteReqCh

		assert.Equal(t, uint64(1), pool.enqueued.from(addr1).length())

		// enqueue last 2
		pool.handleAddRequest(addRequests[4])
		pool.handleAddRequest(addRequests[5])

		assert.Equal(t, uint64(3), pool.enqueued.from(addr1).length())

		// 2nd promotion occurs
		pool.handlePromoteRequest(promReq)
		assert.Equal(t, uint64(0), pool.enqueued.from(addr1).length())
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
			tx:       newDummyTx(addr, 4, 1),
			returnee: true,
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

/* handePromoteRequest cases (single account) */

// account queue (enqueued)
func TestResetHandlerEnqueued(t *testing.T) {
	testCases := []struct {
		name             string
		enqueued         []uint64 // nonces
		newNonce         uint64
		expectedEnqueued uint64
	}{
		{
			name:             "prune some enqueued transactions",
			enqueued:         []uint64{3, 4, 5},
			newNonce:         4,
			expectedEnqueued: 1,
		},
		{
			name:             "prune all enqueued transactions",
			enqueued:         []uint64{2, 5, 6, 8},
			newNonce:         10,
			expectedEnqueued: 0,
		},
		{
			name:             "no enqueued transactions to prune",
			enqueued:         []uint64{9, 10, 12},
			newNonce:         7,
			expectedEnqueued: 3,
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
				for _, nonce := range tc.enqueued {
					go pool.addTx(newDummyTx(addr, nonce, 1))
					pool.handleAddRequest(<-pool.addReqCh)
				}
				assert.Equal(t, uint64(len(tc.enqueued)), pool.enqueued.from(addr).length())
			}

			// align account queue with reset event
			pool.handleResetRequest(resetRequest{
				newNonces: map[types.Address]uint64{
					addr: tc.newNonce,
				},
			})

			assert.Equal(t, tc.expectedEnqueued, pool.enqueued.from(addr).length())
		})
	}
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
			expectedPromoted: 3,
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
				go pool.addTx(newDummyTx(addr, tc.promoted[0], 1))
				go pool.handleAddRequest(<-pool.addReqCh)
				prom := <-pool.promoteReqCh

				// enqueue remaining txs
				tc.promoted = tc.promoted[1:]
				for _, nonce := range tc.promoted {
					go pool.addTx(newDummyTx(addr, nonce, 1))
					pool.handleAddRequest(<-pool.addReqCh)
				}

				// run the first promotion
				pool.handlePromoteRequest(prom)
				assert.Equal(t, initialPromoted, pool.promoted.length())
			}

			// align promoted queue with reset event
			pool.handleResetRequest(resetRequest{
				newNonces: map[types.Address]uint64{
					addr: tc.newNonce,
				},
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
			case req := <-p.resetReqCh:
				go func() {
					p.handleResetRequest(req)
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
		case <-time.After(1 * time.Millisecond):
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
			go pool.addTx(newDummyTx(addr, i, 1))
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
				go pool.addTx(tx)
			}
		}

		waitUntilDone(done)

		assert.Equal(t, uint64(3000), pool.gauge.read())
		assert.Equal(t, uint64(1000), pool.promoted.length())
	})
}

type result struct {
	enqueued map[types.Address]uint64
	promoted uint64
	slots    uint64
}

func TestResetWithHeader(t *testing.T) {

	// 4 accounts
	addr1 := types.Address{0x1}
	addr2 := types.Address{0x2}
	addr3 := types.Address{0x3}
	addr4 := types.Address{0x4}

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
				promoted: 4,
				slots:    4,
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
				addr3: 4,
			},
			expected: result{
				enqueued: map[types.Address]uint64{
					addr1: 2,
					addr2: 2,
					addr3: 3,
				},
				promoted: 0,
				slots:    7,
			},
		},
		{
			name: "reset all queues",
			all: map[types.Address]transactions{
				addr1: {
					newDummyTx(addr1, 3, 3),
					newDummyTx(addr1, 4, 3),
					newDummyTx(addr1, 5, 3),
				},
				addr2: {
					newDummyTx(addr2, 1, 2),
					newDummyTx(addr2, 5, 1),
					newDummyTx(addr2, 6, 1),
					newDummyTx(addr2, 7, 2),
				},
				addr3: {
					newDummyTx(addr3, 0, 1),
					newDummyTx(addr3, 1, 2),
					newDummyTx(addr3, 2, 1),
					newDummyTx(addr3, 4, 3),
					newDummyTx(addr3, 7, 1),
					newDummyTx(addr3, 9, 2),
				},
				addr4: {
					newDummyTx(addr4, 0, 1),
					newDummyTx(addr4, 1, 1),
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
				promoted: 1,
				enqueued: map[types.Address]uint64{
					addr1: 0,
					addr2: 3,
					addr3: 1,
					addr4: 0,
				},
				slots: 0 + 4 + 2 + 1,
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
						go pool.addTx(tx)
					}
				}
				waitUntilDone(done)
			}

			// ResetWitHeader would invoke this handler when a node
			// is building/discovering a new block
			pool.handleResetRequest(resetRequest{tc.newNonces})

			assert.Equal(t, tc.expected.slots, pool.gauge.read())
			assert.Equal(t, tc.expected.promoted, pool.promoted.length())
			for addr, count := range tc.expected.enqueued {
				assert.Equal(t, count, pool.enqueued.from(addr).length())
			}
		})
	}

}

func TestRecoverySingleAccount(t *testing.T) {
	addr1 := types.Address{0x1}
	testCases := []struct {
		name          string
		promoted      transactions
		isRecoverable map[uint64]bool
		expected      result
	}{
		{
			name: "all recovered are promoted again",
			promoted: transactions{
				newDummyTx(addr1, 0, 1),
				newDummyTx(addr1, 1, 2),
				newDummyTx(addr1, 2, 1),
				newDummyTx(addr1, 3, 3),
				newDummyTx(addr1, 4, 2),
			},
			isRecoverable: map[uint64]bool{
				0: true,
				1: true,
				2: true,
				3: true,
				4: true,
			},
			expected: result{
				enqueued: map[types.Address]uint64{
					addr1: 0,
				},
				promoted: 5,
				slots:    9,
			},
		},
		{
			name: "all recovered are enqueued",
			promoted: transactions{
				newDummyTx(addr1, 0, 1),
				newDummyTx(addr1, 1, 2),
				newDummyTx(addr1, 2, 1),
				newDummyTx(addr1, 3, 3),
				newDummyTx(addr1, 4, 2),
			},
			isRecoverable: map[uint64]bool{
				0: false,
				1: true,
				2: true,
				3: true,
				4: true,
			},
			expected: result{
				enqueued: map[types.Address]uint64{
					addr1: 4,
				},
				promoted: 0,
				slots:    8,
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pool, err := newTestPool()
			assert.NoError(t, err)
			pool.AddSigner(&mockSigner{})
			pool.EnableDev()

			done := pool.startTestMode()

			// setup initial state
			{
				for _, tx := range tc.promoted {
					go pool.addTx(tx)
				}
				waitUntilDone(done)
				assert.Equal(t, uint64(5), pool.promoted.length())
			}

			// ibft.writeTransactions()
			func() {
				pool.LockPromoted(true)
				defer pool.UnlockPromoted()

				for {
					tx := pool.Pop()
					if tx == nil {
						break
					}

					switch tc.isRecoverable[tx.Nonce] {
					case true:
						pool.Recover(tx)
					case false:
						pool.RollbackNonce(tx)
					}
				}
			}()

			// pool was handling recoveries in the background...
			waitUntilDone(done)

			assert.Equal(t, tc.expected.slots, pool.gauge.read())
			assert.Equal(t, tc.expected.enqueued[addr1], pool.enqueued.from(addr1).length())
			assert.Equal(t, tc.expected.promoted, pool.promoted.length())
		})
	}
}

func TestRecoveryMultipleAccounts(t *testing.T) {
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

	// 4 accounts
	addr1 := types.Address{0x1}
	addr2 := types.Address{0x2}
	addr3 := types.Address{0x3}
	addr4 := types.Address{0x4}

	testCases := []struct {
		name       string
		nextNonces map[types.Address]uint64
		promoted   transactions
		status     []status
		expected   result
	}{
		{
			name: "all recovered in promoted",
			nextNonces: map[types.Address]uint64{
				addr1: 3,
				addr2: 1,
				addr3: 2,
				addr4: 0,
			},
			promoted: transactions{
				0: newDummyTx(addr1, 0, 1),
				1: newDummyTx(addr2, 0, 2),
				2: newDummyTx(addr1, 1, 1),
				3: newDummyTx(addr3, 0, 2),
				4: newDummyTx(addr3, 1, 2),
				5: newDummyTx(addr1, 2, 1),
			},
			status: []status{
				0: recoverable,
				1: ok,
				2: recoverable,
				3: ok,
				4: recoverable,
				5: recoverable,
			},
			expected: result{
				enqueued: map[types.Address]uint64{
					addr1: 0,
					addr2: 0,
					addr3: 0,
					addr4: 0,
				},
				promoted: 4,
				slots:    5,
			},
		},
		{
			name: "all recovered in enqueued",
			nextNonces: map[types.Address]uint64{
				addr1: 1,
				addr2: 4,
				addr3: 2,
				addr4: 3,
			},
			promoted: transactions{
				0: newDummyTx(addr1, 0, 1),
				1: newDummyTx(addr2, 0, 2),
				2: newDummyTx(addr2, 1, 1),
				3: newDummyTx(addr2, 2, 2),
				4: newDummyTx(addr3, 0, 2),
				5: newDummyTx(addr3, 1, 1),
				6: newDummyTx(addr4, 0, 1),
				7: newDummyTx(addr4, 1, 3),
				8: newDummyTx(addr2, 3, 1),
				9: newDummyTx(addr4, 2, 2),
			},
			status: []status{
				0: ok,
				1: unrecoverable, // rollback addr2
				2: recoverable,
				3: recoverable,
				4: unrecoverable, // rollback addr2
				5: recoverable,
				6: ok,
				7: ok,
				8: recoverable,
				9: ok,
			},
			expected: result{
				enqueued: map[types.Address]uint64{
					addr1: 0,
					addr2: 3,
					addr3: 1,
					addr4: 0,
				},
				promoted: 0,
				slots:    5,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pool, err := newTestPool()
			assert.NoError(t, err)
			pool.AddSigner(&mockSigner{})
			pool.EnableDev()

			done := pool.startTestMode()

			// setup initial state
			{
				for addr, nonce := range tc.nextNonces {
					pool.createAccountOnce(addr)
					pool.nextNonces.store(addr, nonce)
				}
			}

			// mock ibft.writeTransactions()
			func() {

				var recoverables transactions

				for i, tx := range tc.promoted {
					switch tc.status[i] {
					case recoverable:
						recoverables = append(recoverables, tx)
					case unrecoverable:
						pool.RollbackNonce(tx)
					case ok:
						// do nothing
					}

					// "pop" the tx
					tc.promoted = tc.promoted[1:]
				}

				for _, tx := range recoverables {
					pool.Recover(tx)
				}

			}()

			// pool was handling requests
			waitUntilDone(done)

			// assert
			assert.Equal(t, tc.expected.slots, pool.gauge.read())
			assert.Equal(t, tc.expected.promoted, pool.promoted.length())
			for addr := range tc.nextNonces {
				assert.Equal(t, tc.expected.enqueued[addr], pool.enqueued.from(addr).length())
			}
		})
	}
}
