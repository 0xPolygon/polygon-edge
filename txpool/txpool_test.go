package txpool

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang/protobuf/ptypes/any"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/helper/tests"
	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/state/runtime"
	"github.com/0xPolygon/polygon-edge/txpool/proto"
	"github.com/0xPolygon/polygon-edge/types"
)

const (
	defaultPriceLimit         uint64 = 1
	defaultBaseFee            uint64 = 1
	defaultMaxSlots           uint64 = 4096
	defaultMaxAccountEnqueued uint64 = 128
	validGasLimit             uint64 = 4712350
)

// addresses used in tests
var (
	addr1 = types.Address{0x1}
	addr2 = types.Address{0x2}
	addr3 = types.Address{0x3}
	addr4 = types.Address{0x4}
	addr5 = types.Address{0x5}
)

// returns a new valid tx of slots size with the given nonce
func newTx(addr types.Address, nonce, slots uint64) *types.Transaction {
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
func newTestPool(mockStore ...store) (*TxPool, error) {
	return newTestPoolWithSlots(defaultMaxSlots, mockStore...)
}

func newTestPoolWithSlots(maxSlots uint64, mockStore ...store) (*TxPool, error) {
	var storeToUse store
	if len(mockStore) != 0 {
		storeToUse = mockStore[0]
	} else {
		storeToUse = defaultMockStore{
			DefaultHeader: mockHeader,
		}
	}

	return NewTxPool(
		hclog.NewNullLogger(),
		getDefaultEnabledForks(),
		storeToUse,
		nil,
		nil,
		&Config{
			PriceLimit:         defaultPriceLimit,
			MaxSlots:           maxSlots,
			MaxAccountEnqueued: defaultMaxAccountEnqueued,
		},
	)
}

type accountState struct {
	enqueued,
	promoted,
	nextNonce uint64
}

type result struct {
	accounts map[types.Address]accountState
	slots    uint64
}

/* Single account cases (unit tests) */

func TestAddTxErrors(t *testing.T) {
	t.Parallel()

	poolSigner := crypto.NewEIP155Signer(100, true)

	// Generate a private key and address
	defaultKey, defaultAddr := tests.GenerateKeyAndAddr(t)

	setupPool := func() *TxPool {
		pool, err := newTestPool()
		if err != nil {
			t.Fatalf("cannot create txpool - err: %v\n", err)
		}

		pool.SetSigner(poolSigner)

		return pool
	}

	signTx := func(transaction *types.Transaction) *types.Transaction {
		signedTx, signErr := poolSigner.SignTx(transaction, defaultKey)
		if signErr != nil {
			t.Fatalf("Unable to sign transaction, %v", signErr)
		}

		return signedTx
	}

	t.Run("ErrInvalidTxType", func(t *testing.T) {
		t.Parallel()
		pool := setupPool()

		tx := newTx(defaultAddr, 0, 1)
		tx.Type = types.StateTx

		err := pool.addTx(local, signTx(tx))

		assert.ErrorContains(t,
			err,
			ErrInvalidTxType.Error(),
		)
		assert.ErrorContains(t,
			err,
			"state transactions are not expected to be added to the pool",
		)
	})

	t.Run("ErrTxTypeNotSupported London hardfork not enabled", func(t *testing.T) {
		t.Parallel()
		pool := setupPool()
		pool.forks.RemoveFork(chain.London)

		tx := newTx(defaultAddr, 0, 1)
		tx.Type = types.DynamicFeeTx

		err := pool.addTx(local, signTx(tx))

		assert.ErrorContains(t,
			err,
			ErrTxTypeNotSupported.Error(),
		)
		assert.ErrorContains(t,
			err,
			"london hardfork is not enabled",
		)
	})

	t.Run("ErrNegativeValue", func(t *testing.T) {
		t.Parallel()
		pool := setupPool()

		tx := newTx(defaultAddr, 0, 1)
		tx.Value = big.NewInt(-5)

		assert.ErrorIs(t,
			pool.addTx(local, signTx(tx)),
			ErrNegativeValue,
		)
	})

	t.Run("ErrBlockLimitExceeded", func(t *testing.T) {
		t.Parallel()
		pool := setupPool()

		tx := newTx(defaultAddr, 0, 1)
		tx.Value = big.NewInt(1)
		tx.Gas = 10000000000001

		tx = signTx(tx)

		assert.ErrorIs(t,
			pool.addTx(local, tx),
			ErrBlockLimitExceeded,
		)
	})

	t.Run("ErrExtractSignature", func(t *testing.T) {
		t.Parallel()
		pool := setupPool()

		tx := newTx(defaultAddr, 0, 1)

		assert.ErrorIs(t,
			pool.addTx(local, tx),
			ErrExtractSignature,
		)
	})

	t.Run("ErrInvalidSender", func(t *testing.T) {
		t.Parallel()
		pool := setupPool()

		tx := newTx(addr1, 0, 1)

		// Sign with a private key that corresponds
		// to a different address
		tx = signTx(tx)

		assert.ErrorIs(t,
			pool.addTx(local, tx),
			ErrInvalidSender,
		)
	})

	t.Run("ErrUnderpriced", func(t *testing.T) {
		t.Parallel()
		pool := setupPool()
		pool.priceLimit = 1000000

		tx := newTx(defaultAddr, 0, 1) // gasPrice == 1
		tx = signTx(tx)

		assert.ErrorIs(t,
			pool.addTx(local, tx),
			ErrUnderpriced,
		)
	})

	t.Run("ErrInvalidAccountState", func(t *testing.T) {
		t.Parallel()
		pool := setupPool()
		pool.store = faultyMockStore{}

		// nonce is 1000000 so ErrNonceTooLow
		// doesn't get triggered
		tx := newTx(defaultAddr, 1000000, 1)
		tx = signTx(tx)

		assert.ErrorIs(t,
			pool.addTx(local, tx),
			ErrInvalidAccountState,
		)
	})

	t.Run("ErrTxPoolOverflow", func(t *testing.T) {
		t.Parallel()
		pool := setupPool()

		// fill the pool
		pool.gauge.increase(defaultMaxSlots)

		tx := newTx(defaultAddr, 0, 1)
		tx = signTx(tx)

		assert.ErrorIs(t,
			pool.addTx(local, tx),
			ErrTxPoolOverflow,
		)
	})

	t.Run("FillTxPoolToTheLimit", func(t *testing.T) {
		t.Parallel()
		pool := setupPool()

		// fill the pool leaving only 1 slot
		pool.gauge.increase(defaultMaxSlots - 1)

		// create tx requiring 1 slot
		tx := newTx(defaultAddr, 0, 1)
		tx = signTx(tx)

		//	enqueue tx
		assert.NoError(t, pool.addTx(local, tx))
		<-pool.promoteReqCh
	})

	t.Run("ErrIntrinsicGas", func(t *testing.T) {
		t.Parallel()
		pool := setupPool()

		tx := newTx(defaultAddr, 0, 1)
		tx.Gas = 1
		tx = signTx(tx)

		assert.ErrorIs(t,
			pool.addTx(local, tx),
			ErrIntrinsicGas,
		)
	})

	t.Run("ErrAlreadyKnown", func(t *testing.T) {
		t.Parallel()
		pool := setupPool()

		tx := newTx(defaultAddr, 0, 1)
		tx = signTx(tx)

		// send the tx beforehand
		assert.NoError(t, pool.addTx(local, tx))
		<-pool.promoteReqCh

		assert.ErrorIs(t,
			pool.addTx(local, tx),
			ErrAlreadyKnown,
		)
	})

	t.Run("ErrAlreadyKnown", func(t *testing.T) {
		t.Parallel()
		pool := setupPool()

		tx := newTx(defaultAddr, 0, 1)
		tx.GasPrice = big.NewInt(200)
		tx = signTx(tx)

		// send the tx beforehand
		assert.NoError(t, pool.addTx(local, tx))
		<-pool.promoteReqCh

		tx = newTx(defaultAddr, 0, 1)
		tx.GasPrice = big.NewInt(100)
		tx = signTx(tx)

		assert.ErrorIs(t,
			pool.addTx(local, tx),
			ErrReplacementUnderpriced,
		)
	})

	t.Run("ErrOversizedData", func(t *testing.T) {
		t.Parallel()
		pool := setupPool()

		tx := newTx(defaultAddr, 0, 1)

		// set oversized Input field
		data := make([]byte, 989898)
		_, err := rand.Read(data)
		assert.NoError(t, err)

		tx.Input = data
		tx = signTx(tx)

		assert.ErrorIs(t,
			pool.addTx(local, tx),
			ErrOversizedData,
		)
	})

	t.Run("ErrNonceTooLow", func(t *testing.T) {
		t.Parallel()
		pool := setupPool()

		// faultyMockStore.GetNonce() == 99999
		pool.store = faultyMockStore{}
		tx := newTx(defaultAddr, 0, 1)
		tx = signTx(tx)

		assert.ErrorIs(t,
			pool.addTx(local, tx),
			ErrNonceTooLow,
		)
	})

	t.Run("ErrInsufficientFunds", func(t *testing.T) {
		t.Parallel()
		pool := setupPool()

		tx := newTx(defaultAddr, 0, 1)
		tx.GasPrice.SetUint64(1000000000000)
		tx = signTx(tx)

		assert.ErrorIs(t,
			pool.addTx(local, tx),
			ErrInsufficientFunds,
		)
	})
}

func TestPruneAccountsWithNonceHoles(t *testing.T) {
	t.Parallel()

	t.Run(
		"no enqueued to prune",
		func(t *testing.T) {
			t.Parallel()

			pool, err := newTestPool()
			assert.NoError(t, err)
			pool.SetSigner(&mockSigner{})

			pool.getOrCreateAccount(addr1)

			assert.Equal(t, uint64(0), pool.gauge.read())
			assert.Equal(t, uint64(0), pool.accounts.get(addr1).getNonce())
			assert.Equal(t, uint64(0), pool.accounts.get(addr1).enqueued.length())

			pool.pruneAccountsWithNonceHoles()

			assert.Equal(t, uint64(0), pool.gauge.read())
			assert.Equal(t, uint64(0), pool.accounts.get(addr1).getNonce())
			assert.Equal(t, uint64(0), pool.accounts.get(addr1).enqueued.length())
		},
	)

	t.Run(
		"skip valid account",
		func(t *testing.T) {
			t.Parallel()

			pool, err := newTestPool()
			assert.NoError(t, err)
			pool.SetSigner(&mockSigner{})

			tx := newTx(addr1, 0, 1)

			// enqueue tx
			assert.NoError(t, pool.addTx(local, tx))
			acc := pool.accounts.get(addr1)
			<-pool.promoteReqCh

			assert.Equal(t, uint64(1), pool.gauge.read())
			assert.Equal(t, uint64(1), pool.accounts.get(addr1).enqueued.length())

			//	assert no nonce hole
			assert.Equal(t,
				pool.accounts.get(addr1).getNonce(),
				pool.accounts.get(addr1).enqueued.peek().Nonce,
			)

			pool.pruneAccountsWithNonceHoles()

			assert.Equal(t, uint64(1), pool.gauge.read())
			assert.Equal(t, uint64(1), pool.accounts.get(addr1).enqueued.length())

			assert.Equal(t, int(1), len(acc.nonceToTx.mapping))
		},
	)

	t.Run(
		"prune nonce hole account",
		func(t *testing.T) {
			t.Parallel()

			pool, err := newTestPool()
			assert.NoError(t, err)
			pool.SetSigner(&mockSigner{})

			tx := newTx(addr1, 5, 1)

			//	enqueue tx
			assert.NoError(t, pool.addTx(local, tx))
			assert.Equal(t, uint64(1), pool.gauge.read())
			assert.Equal(t, uint64(1), pool.accounts.get(addr1).enqueued.length())

			//	assert nonce hole
			assert.NotEqual(t,
				pool.accounts.get(addr1).getNonce(),
				pool.accounts.get(addr1).enqueued.peek().Nonce,
			)

			pool.pruneAccountsWithNonceHoles()

			assert.Equal(t, uint64(0), pool.gauge.read())
			assert.Equal(t, uint64(0), pool.accounts.get(addr1).enqueued.length())

			acc := pool.accounts.get(addr1)

			assert.Equal(t, int(0), len(acc.nonceToTx.mapping))
		},
	)
}

func TestAddTxHighPressure(t *testing.T) {
	t.Parallel()

	t.Run(
		"pruning handler is signaled",
		func(t *testing.T) {
			t.Parallel()

			pool, err := newTestPool()
			assert.NoError(t, err)
			pool.SetSigner(&mockSigner{})

			//	mock high pressure
			slots := 1 + (highPressureMark*pool.gauge.max)/100
			pool.gauge.increase(slots)

			//	enqueue tx
			go func() {
				assert.NoError(t,
					pool.addTx(local, newTx(addr1, 0, 1)),
				)
			}()

			//	pick up signal
			_, ok := <-pool.pruneCh
			assert.True(t, ok)
		},
	)

	t.Run(
		"reject tx with nonce not matching expected",
		func(t *testing.T) {
			t.Parallel()

			pool, err := newTestPool()
			assert.NoError(t, err)
			pool.SetSigner(&mockSigner{})

			pool.getOrCreateAccount(addr1)
			pool.accounts.get(addr1).nextNonce = 5

			//	mock high pressure
			slots := 1 + (highPressureMark*pool.gauge.max)/100
			pool.gauge.increase(slots)

			assert.ErrorIs(t,
				ErrRejectFutureTx,
				pool.addTx(local, newTx(addr1, 8, 1)),
			)

			acc := pool.accounts.get(addr1)

			assert.Equal(t, int(0), len(acc.nonceToTx.mapping))
		},
	)

	t.Run(
		"accept tx with expected nonce during high gauge level",
		func(t *testing.T) {
			t.Parallel()

			pool, err := newTestPool()
			assert.NoError(t, err)
			pool.SetSigner(&mockSigner{})

			pool.getOrCreateAccount(addr1)
			pool.accounts.get(addr1).nextNonce = 5

			//	mock high pressure
			slots := 1 + (highPressureMark*pool.gauge.max)/100
			println("slots", slots, "max", pool.gauge.max)
			pool.gauge.increase(slots)

			tx := newTx(addr1, 5, 1)
			assert.NoError(t, pool.addTx(local, tx))

			_, exists := pool.index.get(tx.Hash)
			assert.True(t, exists)

			acc := pool.accounts.get(addr1)

			assert.Equal(t, int(1), len(acc.nonceToTx.mapping))
		},
	)
}

func TestAddGossipTx(t *testing.T) {
	t.Parallel()

	key, sender := tests.GenerateKeyAndAddr(t)
	signer := crypto.NewEIP155Signer(100, true)
	tx := newTx(types.ZeroAddress, 1, 1)

	t.Run("node is a validator", func(t *testing.T) {
		t.Parallel()

		pool, err := newTestPool()
		assert.NoError(t, err)
		pool.SetSigner(signer)

		pool.SetSealing(true)

		signedTx, err := signer.SignTx(tx, key)
		if err != nil {
			t.Fatalf("cannot sign transaction - err: %v", err)
		}

		// send tx
		protoTx := &proto.Txn{
			Raw: &any.Any{
				Value: signedTx.MarshalRLP(),
			},
		}
		pool.addGossipTx(protoTx, "")

		assert.Equal(t, uint64(1), pool.accounts.get(sender).enqueued.length())
	})

	t.Run("node is a non validator", func(t *testing.T) {
		t.Parallel()

		pool, err := newTestPool()
		assert.NoError(t, err)
		pool.SetSigner(signer)

		pool.SetSealing(false)

		pool.getOrCreateAccount(sender)

		signedTx, err := signer.SignTx(tx, key)
		if err != nil {
			t.Fatalf("cannot sign transaction - err: %v", err)
		}

		// send tx
		protoTx := &proto.Txn{
			Raw: &any.Any{
				Value: signedTx.MarshalRLP(),
			},
		}
		pool.addGossipTx(protoTx, "")

		assert.Equal(t, uint64(0), pool.accounts.get(sender).enqueued.length())
	})
}

func TestDropKnownGossipTx(t *testing.T) {
	t.Parallel()

	pool, err := newTestPool()
	assert.NoError(t, err)
	pool.SetSigner(&mockSigner{})

	tx := newTx(addr1, 1, 1)

	// send tx as local
	assert.NoError(t, pool.addTx(local, tx))

	_, exists := pool.index.get(tx.Hash)
	assert.True(t, exists)

	// send tx as gossip (will be discarded)
	assert.ErrorIs(t,
		pool.addTx(gossip, tx.Copy()),
		ErrAlreadyKnown,
	)

	acc := pool.accounts.get(addr1)

	assert.Equal(t, int(1), len(acc.nonceToTx.mapping))
}

func TestEnqueueHandler(t *testing.T) {
	t.Parallel()

	t.Run(
		"enqueue new tx with higher nonce",
		func(t *testing.T) {
			t.Parallel()

			pool, err := newTestPool()
			assert.NoError(t, err)
			pool.SetSigner(&mockSigner{})

			// send higher nonce tx
			err = pool.addTx(local, newTx(addr1, 10, 1)) // 10 > 0
			assert.NoError(t, err)

			assert.Equal(t, uint64(1), pool.gauge.read())
			assert.Equal(t, uint64(1), pool.accounts.get(addr1).enqueued.length())
		},
	)

	t.Run(
		"reject new tx with low nonce",
		func(t *testing.T) {
			t.Parallel()

			pool, err := newTestPool()
			assert.NoError(t, err)
			pool.SetSigner(&mockSigner{})

			// setup prestate
			acc := pool.getOrCreateAccount(addr1)
			acc.setNonce(20)

			// send tx
			go func() {
				err := pool.addTx(local, newTx(addr1, 10, 1)) // 10 < 20
				assert.EqualError(t, err, "nonce too low")
			}()

			assert.Equal(t, uint64(0), pool.gauge.read())
			assert.Equal(t, uint64(0), pool.accounts.get(addr1).enqueued.length())

			acc.nonceToTx.lock()
			assert.Equal(t, int(0), len(acc.nonceToTx.mapping))
			acc.nonceToTx.unlock()
		},
	)

	t.Run(
		"signal promotion for new tx with expected nonce",
		func(t *testing.T) {
			t.Parallel()

			pool, err := newTestPool()
			assert.NoError(t, err)
			pool.SetSigner(&mockSigner{})

			// send tx
			err = pool.addTx(local, newTx(addr1, 0, 1)) // 0 == 0
			assert.NoError(t, err)

			// catch pending promotion
			<-pool.promoteReqCh

			assert.Equal(t, uint64(1), pool.gauge.read())
			assert.Equal(t, uint64(1), pool.accounts.get(addr1).enqueued.length())
			assert.Equal(t, uint64(0), pool.accounts.get(addr1).promoted.length())

			acc := pool.accounts.get(addr1)
			acc.nonceToTx.lock()
			assert.Equal(t, int(1), len(acc.nonceToTx.mapping))
			acc.nonceToTx.unlock()
		},
	)

	t.Run(
		"reject new tx when enqueued is full",
		func(t *testing.T) {
			t.Parallel()

			fillEnqueued := func(pool *TxPool, num uint64) {
				//	first tx will signal promotion, grab the signal
				//	but don't execute the handler
				err := pool.addTx(local, newTx(addr1, 0, 1))
				assert.NoError(t, err)

				// catch pending promotion
				<-pool.promoteReqCh

				for i := uint64(1); i < num; i++ {
					err := pool.addTx(local, newTx(addr1, i, 1))
					assert.NoError(t, err)
				}
			}

			pool, err := newTestPool()
			assert.NoError(t, err)
			pool.SetSigner(&mockSigner{})

			//	mock full enqueued
			pool.accounts.maxEnqueuedLimit = 1
			fillEnqueued(pool, 1)

			assert.Equal(t, uint64(1), pool.accounts.get(addr1).enqueued.length())
			assert.Equal(t, uint64(1), pool.gauge.read())
			assert.Equal(t, uint64(0), pool.accounts.get(addr1).getNonce())

			//	send next expected tx
			go func() {
				err := pool.addTx(local, newTx(addr1, 1, 1))
				assert.True(t, errors.Is(err, ErrMaxEnqueuedLimitReached))
			}()

			//	assert the transaction was rejected
			assert.Equal(t, uint64(1), pool.accounts.get(addr1).enqueued.length())
			assert.Equal(t, uint64(1), pool.gauge.read())
			assert.Equal(t, uint64(0), pool.accounts.get(addr1).getNonce())

			acc := pool.accounts.get(addr1)
			acc.nonceToTx.lock()
			assert.Equal(t, int(1), len(acc.nonceToTx.mapping))
			acc.nonceToTx.unlock()
		},
	)

	t.Run(
		"accept new tx with nextNonce when enqueued is full",
		func(t *testing.T) {
			t.Parallel()

			pool, err := newTestPool()
			assert.NoError(t, err)
			pool.SetSigner(&mockSigner{})

			// mock full enqueued
			pool.accounts.maxEnqueuedLimit = 10

			// add 10 transaction in txpool i.e. max enqueued transactions
			for i := uint64(1); i <= 10; i++ {
				err := pool.addTx(local, newTx(addr1, i, 1))
				assert.NoError(t, err)
			}

			assert.Equal(t, uint64(10), pool.accounts.get(addr1).enqueued.length())
			assert.Equal(t, uint64(0), pool.accounts.get(addr1).promoted.length())
			assert.Equal(t, uint64(0), pool.accounts.get(addr1).getNonce())

			err = pool.addTx(local, newTx(addr1, 11, 1))
			assert.True(t, errors.Is(err, ErrMaxEnqueuedLimitReached))

			// add the transaction with nextNonce i.e. nonce=0
			err = pool.addTx(local, newTx(addr1, uint64(0), 1))
			assert.NoError(t, err)

			pool.handlePromoteRequest(<-pool.promoteReqCh)

			assert.Equal(t, uint64(0), pool.accounts.get(addr1).enqueued.length())
			assert.Equal(t, uint64(11), pool.accounts.get(addr1).promoted.length())
			assert.Equal(t, uint64(11), pool.accounts.get(addr1).getNonce())
		},
	)
}

func TestAddTx(t *testing.T) {
	t.Parallel()

	t.Run(
		"return underpriced for cheaper tx when pricier tx exists in enqueued",
		func(t *testing.T) {
			t.Parallel()

			// helper
			newPricedTx := func(
				addr types.Address,
				nonce,
				gasPrice,
				slots uint64,
			) *types.Transaction {
				tx := newTx(addr, nonce, slots)
				tx.GasPrice.SetUint64(gasPrice)

				return tx
			}

			pool, err := newTestPool()
			assert.NoError(t, err)
			pool.SetSigner(&mockSigner{})

			tx1 := newPricedTx(addr1, 0, 10, 2)
			tx2 := newPricedTx(addr1, 0, 20, 3)

			// add the transactions
			assert.NoError(t, pool.addTx(local, tx2))

			_, exists := pool.index.get(tx2.Hash)
			assert.True(t, exists)

			// check the account nonce before promoting
			assert.Equal(t, uint64(0), pool.accounts.get(addr1).getNonce())

			assert.ErrorIs(t, pool.addTx(local, tx1), ErrReplacementUnderpriced)

			//	execute the enqueue handlers
			<-pool.promoteReqCh

			// at this point the pointer of the first tx should be overwritten by the second pricier tx
			_, exists = pool.index.get(tx2.Hash)
			assert.True(t, exists)

			assert.Len(t, pool.index.all, int(1))

			assert.Equal(t, uint64(0), pool.accounts.get(addr1).getNonce())
			assert.Equal(t, uint64(1), pool.accounts.get(addr1).enqueued.length())
			assert.Equal(t, uint64(0), pool.accounts.get(addr1).promoted.length())
			assert.Equal(
				t,
				slotsRequired(tx2),
				pool.gauge.read(),
			)

			acc := pool.accounts.get(addr1)
			acc.nonceToTx.lock()
			assert.Equal(t, int(1), len(acc.nonceToTx.mapping))
			acc.nonceToTx.unlock()
		},
	)

	t.Run(
		"return underpriced for cheaper tx when pricier tx exists in promoted",
		func(t *testing.T) {
			t.Parallel()

			// helper
			newPricedTx := func(
				addr types.Address,
				nonce,
				gasPrice,
				slots uint64,
			) *types.Transaction {
				tx := newTx(addr, nonce, slots)
				tx.GasPrice.SetUint64(gasPrice)

				return tx
			}

			pool, err := newTestPool()
			assert.NoError(t, err)
			pool.SetSigner(&mockSigner{})

			tx1 := newPricedTx(addr1, 0, 10, 2)
			tx2 := newPricedTx(addr1, 0, 20, 3)

			// add the transactions
			assert.NoError(t, pool.addTx(local, tx2))

			_, exists := pool.index.get(tx2.Hash)
			assert.True(t, exists)

			// check the account nonce before promoting
			assert.Equal(t, uint64(0), pool.accounts.get(addr1).getNonce())

			//	execute the enqueue handlers
			promoteReq := <-pool.promoteReqCh
			pool.handlePromoteRequest(promoteReq)

			assert.Equal(t, uint64(1), pool.accounts.get(addr1).getNonce())
			assert.Equal(t, uint64(0), pool.accounts.get(addr1).enqueued.length())
			assert.Equal(t, uint64(1), pool.accounts.get(addr1).promoted.length())
			assert.Equal(
				t,
				slotsRequired(tx2),
				pool.gauge.read(),
			)

			assert.ErrorIs(t, pool.addTx(local, tx1), ErrReplacementUnderpriced)

			acc := pool.accounts.get(addr1)
			acc.nonceToTx.lock()
			assert.Equal(t, int(1), len(acc.nonceToTx.mapping))
			acc.nonceToTx.unlock()
		},
	)

	t.Run(
		"addTx handler discards cheaper tx",
		func(t *testing.T) {
			t.Parallel()

			// helper
			newPricedTx := func(
				addr types.Address,
				nonce,
				gasPrice,
				slots uint64,
			) *types.Transaction {
				tx := newTx(addr, nonce, slots)
				tx.GasPrice.SetUint64(gasPrice)

				return tx
			}

			pool, err := newTestPool()
			assert.NoError(t, err)
			pool.SetSigner(&mockSigner{})

			tx1 := newPricedTx(addr1, 0, 10, 2)
			tx2 := newPricedTx(addr1, 0, 20, 3)

			// add the transactions
			assert.NoError(t, pool.addTx(local, tx1))
			assert.NoError(t, pool.addTx(local, tx2))

			_, exists := pool.index.get(tx1.Hash)
			assert.False(t, exists)

			_, exists = pool.index.get(tx2.Hash)
			assert.True(t, exists)

			// check the account nonce before promoting
			assert.Equal(t, uint64(0), pool.accounts.get(addr1).getNonce())

			//	execute the enqueue handlers
			promReq1 := <-pool.promoteReqCh
			promReq2 := <-pool.promoteReqCh

			// at this point the pointer of the first tx should be overwritten by the second pricier tx
			_, exists = pool.index.get(tx2.Hash)
			assert.True(t, exists)

			assert.Len(t, pool.index.all, int(1))

			assert.Equal(t, uint64(0), pool.accounts.get(addr1).getNonce())
			assert.Equal(t, uint64(1), pool.accounts.get(addr1).enqueued.length())
			assert.Equal(t, uint64(0), pool.accounts.get(addr1).promoted.length())
			assert.Equal(
				t,
				slotsRequired(tx2),
				pool.gauge.read(),
			)

			// promote the second Tx and remove the first Tx
			pool.handlePromoteRequest(promReq1)

			assert.Equal(t, uint64(1), pool.accounts.get(addr1).getNonce())
			assert.Equal(t, uint64(0), pool.accounts.get(addr1).enqueued.length()) // should be empty
			assert.Equal(t, uint64(1), pool.accounts.get(addr1).promoted.length())

			_, exists = pool.index.get(tx2.Hash)
			assert.True(t, exists)

			assert.Equal(t, len(pool.index.all), int(1))

			assert.Equal(
				t,
				slotsRequired(tx2),
				pool.gauge.read(),
			)

			// should do nothing in the 2nd promotion
			pool.handlePromoteRequest(promReq2)

			assert.Equal(t, uint64(1), pool.accounts.get(addr1).getNonce())
			assert.Equal(t, uint64(0), pool.accounts.get(addr1).enqueued.length())
			assert.Equal(t, uint64(1), pool.accounts.get(addr1).promoted.length())

			// because the *tx1 and *tx2 now contain the same hash we only need to check for *tx2 existence
			_, exists = pool.index.get(tx2.Hash)
			assert.True(t, exists)

			assert.Equal(t, len(pool.index.all), int(1))

			assert.Equal(
				t,
				slotsRequired(tx2),
				pool.gauge.read(),
			)

			acc := pool.accounts.get(addr1)
			acc.nonceToTx.lock()
			assert.Equal(t, int(1), len(acc.nonceToTx.mapping))
			acc.nonceToTx.unlock()
		},
	)

	t.Run(
		"addTx discards cheaper tx from enqueued",
		func(t *testing.T) {
			t.Parallel()

			// helper
			newPricedTx := func(
				addr types.Address,
				nonce,
				gasPrice,
				slots uint64,
			) *types.Transaction {
				tx := newTx(addr, nonce, slots)
				tx.GasPrice.SetUint64(gasPrice)

				return tx
			}

			pool, err := newTestPool()
			assert.NoError(t, err)
			pool.SetSigner(&mockSigner{})

			tx1 := newPricedTx(addr1, 0, 10, 2)
			tx2 := newPricedTx(addr1, 0, 20, 3)

			// add the transactions
			assert.NoError(t, pool.addTx(local, tx1))
			assert.NoError(t, pool.addTx(local, tx2))

			acc := pool.accounts.get(addr1)
			assert.NotNil(t, acc)

			_, exists := pool.index.get(tx1.Hash)
			assert.False(t, exists)

			_, exists = pool.index.get(tx2.Hash)
			assert.True(t, exists)

			maptx2 := acc.nonceToTx.get(tx2.Nonce)
			nonceMapLength := len(acc.nonceToTx.mapping)

			assert.NotNil(t, maptx2)
			assert.Equal(t, tx2, maptx2)
			assert.Equal(t, int(1), nonceMapLength)

			assert.Equal(t, len(pool.index.all), int(1))

			assert.Equal(
				t,
				slotsRequired(tx2),
				pool.gauge.read(),
			)

			assert.Equal(t, uint64(0), pool.accounts.get(addr1).getNonce())
			assert.Equal(t, uint64(1), pool.accounts.get(addr1).enqueued.length())
			assert.Equal(t, uint64(0), pool.accounts.get(addr1).promoted.length())
		},
	)

	t.Run(
		"addTx discards cheaper tx from promoted",
		func(t *testing.T) {
			t.Parallel()

			// helper
			newPricedTx := func(
				addr types.Address,
				nonce,
				gasPrice,
				slots uint64,
			) *types.Transaction {
				tx := newTx(addr, nonce, slots)
				tx.GasPrice.SetUint64(gasPrice)

				return tx
			}

			pool, err := newTestPool()
			assert.NoError(t, err)
			pool.SetSigner(&mockSigner{})

			tx1 := newPricedTx(addr1, 0, 10, 2)
			tx2 := newPricedTx(addr1, 0, 20, 3)

			// add the transactions
			assert.NoError(t, pool.addTx(local, tx1))

			acc := pool.accounts.get(addr1)
			assert.NotNil(t, acc)

			promReq1 := <-pool.promoteReqCh
			pool.handlePromoteRequest(promReq1)

			assert.Equal(t, len(pool.index.all), int(1))

			assert.Equal(
				t,
				slotsRequired(tx1),
				pool.gauge.read(),
			)

			assert.Equal(t, uint64(1), pool.accounts.get(addr1).getNonce())
			assert.Equal(t, uint64(0), pool.accounts.get(addr1).enqueued.length())
			assert.Equal(t, uint64(1), pool.accounts.get(addr1).promoted.length())

			_, exists := pool.index.get(tx1.Hash)
			assert.True(t, exists)

			_, exists = pool.index.get(tx2.Hash)
			assert.False(t, exists)

			assert.NoError(t, pool.addTx(local, tx2))

			maptx2 := acc.nonceToTx.get(tx2.Nonce)
			nonceMapLength := len(acc.nonceToTx.mapping)

			assert.NotNil(t, maptx2)
			assert.Equal(t, tx2, maptx2)
			assert.Equal(t, int(1), nonceMapLength)

			_, exists = pool.index.get(tx1.Hash)
			assert.False(t, exists)

			_, exists = pool.index.get(tx2.Hash)
			assert.True(t, exists)

			assert.Equal(t, len(pool.index.all), int(1))

			assert.Equal(
				t,
				slotsRequired(tx2),
				pool.gauge.read(),
			)

			assert.Equal(t, uint64(1), pool.accounts.get(addr1).getNonce())
			assert.Equal(t, uint64(0), pool.accounts.get(addr1).enqueued.length())
			assert.Equal(t, uint64(1), pool.accounts.get(addr1).promoted.length())
		},
	)
}

func TestPromoteHandler(t *testing.T) {
	t.Parallel()

	t.Run("nothing to promote", func(t *testing.T) {
		/* This test demonstrates that if some promotion handler
		got its job done by a previous one, it will not perform any logic
		by doing an early return. */
		t.Parallel()

		pool, err := newTestPool()
		assert.NoError(t, err)
		pool.SetSigner(&mockSigner{})

		// fake a promotion signal
		signalPromotion := func() {
			pool.promoteReqCh <- promoteRequest{account: addr1}
		}

		// fresh account (queues are empty)
		acc := pool.getOrCreateAccount(addr1)
		acc.setNonce(7)

		// fake a promotion
		go signalPromotion()
		pool.handlePromoteRequest(<-pool.promoteReqCh)
		assert.Equal(t, uint64(0), pool.accounts.get(addr1).enqueued.length())
		assert.Equal(t, uint64(0), pool.accounts.get(addr1).promoted.length())

		// enqueue higher nonce tx
		tx := newTx(addr1, 10, 1)
		err = pool.addTx(local, tx)
		assert.NoError(t, err)

		assert.Equal(t, uint64(1), pool.accounts.get(addr1).enqueued.length())
		assert.Equal(t, uint64(0), pool.accounts.get(addr1).promoted.length())

		mapLen := len(acc.nonceToTx.mapping)
		maptx := acc.nonceToTx.get(tx.Nonce)

		assert.Equal(t, int(1), mapLen)
		assert.Equal(t, tx, maptx)

		// fake a promotion
		go signalPromotion()
		pool.handlePromoteRequest(<-pool.promoteReqCh)
		assert.Equal(t, uint64(1), pool.accounts.get(addr1).enqueued.length())
		assert.Equal(t, uint64(0), pool.accounts.get(addr1).promoted.length())

		mapLen = len(acc.nonceToTx.mapping)
		maptx = acc.nonceToTx.get(tx.Nonce)

		assert.Equal(t, int(1), mapLen)
		assert.Equal(t, tx, maptx)
	})

	t.Run("promote one tx", func(t *testing.T) {
		t.Parallel()

		pool, err := newTestPool()
		assert.NoError(t, err)
		pool.SetSigner(&mockSigner{})

		tx := newTx(addr1, 0, 1)
		err = pool.addTx(local, tx)
		assert.NoError(t, err)

		acc := pool.accounts.get(addr1)
		assert.NotNil(t, acc)

		mapLen := len(acc.nonceToTx.mapping)
		maptx := acc.nonceToTx.get(tx.Nonce)

		assert.Equal(t, int(1), mapLen)
		assert.Equal(t, tx, maptx)

		// tx enqueued -> promotion signaled
		pool.handlePromoteRequest(<-pool.promoteReqCh)

		assert.Equal(t, uint64(1), pool.gauge.read())
		assert.Equal(t, uint64(1), pool.accounts.get(addr1).getNonce())

		assert.Equal(t, uint64(0), pool.accounts.get(addr1).enqueued.length())
		assert.Equal(t, uint64(1), pool.accounts.get(addr1).promoted.length())

		mapLen = len(acc.nonceToTx.mapping)
		maptx = acc.nonceToTx.get(tx.Nonce)

		assert.Equal(t, int(1), mapLen)
		assert.Equal(t, tx, maptx)
	})

	t.Run("promote several txs", func(t *testing.T) {
		t.Parallel()
		/* This example illustrates the flexibility of the handlers:
		One promotion handler can be executed at any time after it
		was invoked (when the runtime decides), resulting in promotion
		of several enqueued txs. */

		pool, err := newTestPool()
		assert.NoError(t, err)
		pool.SetSigner(&mockSigner{})

		txs := make([]*types.Transaction, 10)

		for i := 0; i < 10; i++ {
			txs[i] = newTx(addr1, uint64(i), 1)
		}

		// send the first (expected) tx -> signals promotion
		err = pool.addTx(local, txs[0]) // 0 == 0
		assert.NoError(t, err)

		// save the promotion handler
		req := <-pool.promoteReqCh

		// send the remaining txs (all will be enqueued)
		for i := 1; i < 10; i++ {
			err := pool.addTx(local, txs[i])
			assert.NoError(t, err)
		}

		acc := pool.accounts.get(addr1)

		for _, tx := range txs {
			maptx := acc.nonceToTx.get(tx.Nonce)
			assert.Equal(t, tx, maptx)
		}

		assert.Equal(t, int(10), len(acc.nonceToTx.mapping))

		// verify all 10 are enqueued
		assert.Equal(t, uint64(10), pool.accounts.get(addr1).enqueued.length())
		assert.Equal(t, uint64(0), pool.accounts.get(addr1).promoted.length())

		// execute the promotion handler
		pool.handlePromoteRequest(req)

		assert.Equal(t, uint64(10), pool.gauge.read())
		assert.Equal(t, uint64(10), pool.accounts.get(addr1).getNonce())

		assert.Equal(t, uint64(0), pool.accounts.get(addr1).enqueued.length())
		assert.Equal(t, uint64(10), pool.accounts.get(addr1).promoted.length())

		for _, tx := range txs {
			maptx := acc.nonceToTx.get(tx.Nonce)
			assert.Equal(t, tx, maptx)
		}

		assert.Equal(t, int(10), len(acc.nonceToTx.mapping))
	})

	t.Run("one tx -> one promotion", func(t *testing.T) {
		/* In this scenario, each received tx will be instantly promoted.
		All txs are sent in the order of expected nonce. */
		t.Parallel()

		pool, err := newTestPool()
		assert.NoError(t, err)
		pool.SetSigner(&mockSigner{})

		txs := make([]*types.Transaction, 20)

		for i := 0; i < 20; i++ {
			txs[i] = newTx(addr1, uint64(i), 1)
		}

		for _, tx := range txs {
			err := pool.addTx(local, tx)
			assert.NoError(t, err)
			pool.handlePromoteRequest(<-pool.promoteReqCh)
		}

		acc := pool.accounts.get(addr1)

		for _, tx := range txs {
			maptx := acc.nonceToTx.get(tx.Nonce)
			assert.Equal(t, tx, maptx)
		}

		assert.Equal(t, int(20), len(acc.nonceToTx.mapping))

		assert.Equal(t, uint64(20), pool.gauge.read())
		assert.Equal(t, uint64(20), pool.accounts.get(addr1).getNonce())

		assert.Equal(t, uint64(0), pool.accounts.get(addr1).enqueued.length())
		assert.Equal(t, uint64(20), pool.accounts.get(addr1).promoted.length())
	})
}

func TestResetAccount(t *testing.T) {
	t.Parallel()

	t.Run("reset promoted", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name     string
			txs      []*types.Transaction
			newNonce uint64
			expected result
		}{
			{
				name: "prune all txs with low nonce",
				txs: []*types.Transaction{
					newTx(addr1, 0, 1),
					newTx(addr1, 1, 1),
					newTx(addr1, 2, 1),
					newTx(addr1, 3, 1),
					newTx(addr1, 4, 1),
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
				name: "no low nonce txs to prune",
				txs: []*types.Transaction{
					newTx(addr1, 2, 1),
					newTx(addr1, 3, 1),
					newTx(addr1, 4, 1),
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
				name: "prune some txs with low nonce",
				txs: []*types.Transaction{
					newTx(addr1, 7, 1),
					newTx(addr1, 8, 1),
					newTx(addr1, 9, 1),
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
			test := test
			t.Run(test.name, func(t *testing.T) {
				t.Parallel()

				pool, err := newTestPool()
				assert.NoError(t, err)
				pool.SetSigner(&mockSigner{})

				// setup prestate
				acc := pool.getOrCreateAccount(addr1)
				acc.setNonce(test.txs[0].Nonce)

				err = pool.addTx(local, test.txs[0])
				assert.NoError(t, err)

				// save the promotion
				req := <-pool.promoteReqCh

				// enqueue remaining
				for i, tx := range test.txs {
					if i == 0 {
						// first was handled
						continue
					}

					err := pool.addTx(local, tx)
					assert.NoError(t, err)
				}

				pool.handlePromoteRequest(req)

				assert.Equal(t, uint64(0), acc.enqueued.length())
				assert.Equal(t, uint64(len(test.txs)), acc.promoted.length())
				assert.Equal(t, len(test.txs), len(acc.nonceToTx.mapping))

				pool.resetAccounts(map[types.Address]uint64{
					addr1: test.newNonce,
				})

				assert.Equal(t,
					int(test.expected.accounts[addr1].promoted+test.expected.accounts[addr1].enqueued),
					len(acc.nonceToTx.mapping))

				assert.Equal(t, test.expected.slots, pool.gauge.read())
				assert.Equal(t, // enqueued
					test.expected.accounts[addr1].enqueued,
					pool.accounts.get(addr1).enqueued.length())
				assert.Equal(t, // promoted
					test.expected.accounts[addr1].promoted,
					pool.accounts.get(addr1).promoted.length())
			})
		}
	})

	t.Run("reset enqueued", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name     string
			txs      []*types.Transaction
			newNonce uint64
			expected result
			signal   bool // flag indicating whether reset will cause a promotion
		}{
			{
				name: "prune all txs with low nonce",
				txs: []*types.Transaction{
					newTx(addr1, 5, 1),
					newTx(addr1, 6, 1),
					newTx(addr1, 7, 1),
					newTx(addr1, 8, 1),
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
				name: "no low nonce txs to prune",
				txs: []*types.Transaction{
					newTx(addr1, 2, 1),
					newTx(addr1, 3, 1),
					newTx(addr1, 4, 1),
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
				name: "prune some txs with low nonce",
				txs: []*types.Transaction{
					newTx(addr1, 4, 1),
					newTx(addr1, 5, 1),
					newTx(addr1, 8, 1),
					newTx(addr1, 9, 1),
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
				name:   "pruning low nonce signals promotion",
				signal: true,
				txs: []*types.Transaction{
					newTx(addr1, 8, 1),
					newTx(addr1, 9, 1),
					newTx(addr1, 10, 1),
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
			test := test
			t.Run(test.name, func(t *testing.T) {
				t.Parallel()

				pool, err := newTestPool()
				assert.NoError(t, err)
				pool.SetSigner(&mockSigner{})

				// setup prestate
				for _, tx := range test.txs {
					err := pool.addTx(local, tx)
					assert.NoError(t, err)
				}

				acc := pool.accounts.get(addr1)
				assert.Equal(t, len(test.txs), len(acc.nonceToTx.mapping))

				assert.Equal(t, uint64(len(test.txs)), pool.accounts.get(addr1).enqueued.length())
				assert.Equal(t, uint64(0), pool.accounts.get(addr1).promoted.length())

				if test.signal {
					go pool.resetAccounts(map[types.Address]uint64{
						addr1: test.newNonce,
					})
					pool.handlePromoteRequest(<-pool.promoteReqCh)
				} else {
					pool.resetAccounts(map[types.Address]uint64{
						addr1: test.newNonce,
					})
				}

				// not sure if this lock is needed
				// but because resetAccounts() runs in a separate goroutine if it is triggered
				// it is safer to have a lock on the nonce map
				acc.nonceToTx.lock()

				assert.Equal(t, int(test.expected.accounts[addr1].enqueued+test.expected.accounts[addr1].promoted), len(acc.nonceToTx.mapping))

				acc.nonceToTx.unlock()

				assert.Equal(t, test.expected.slots, pool.gauge.read())
				assert.Equal(t, // enqueued
					test.expected.accounts[addr1].enqueued,
					pool.accounts.get(addr1).enqueued.length())
				assert.Equal(t, // promoted
					test.expected.accounts[addr1].promoted,
					pool.accounts.get(addr1).promoted.length())
			})
		}
	})

	t.Run("reset enqueued and promoted", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name     string
			txs      []*types.Transaction
			newNonce uint64
			expected result
			signal   bool // flag indicating whether reset will cause a promotion
		}{
			{
				name: "prune all txs with low nonce",
				txs: []*types.Transaction{
					// promoted
					newTx(addr1, 0, 1),
					newTx(addr1, 1, 1),
					newTx(addr1, 2, 1),
					newTx(addr1, 3, 1),
					// enqueued
					newTx(addr1, 5, 1),
					newTx(addr1, 6, 1),
					newTx(addr1, 8, 1),
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
				name: "no low nonce txs to prune",
				txs: []*types.Transaction{
					// promoted
					newTx(addr1, 5, 1),
					newTx(addr1, 6, 1),
					// enqueued
					newTx(addr1, 9, 1),
					newTx(addr1, 10, 1),
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
				name: "prune all promoted and 1 enqueued",
				txs: []*types.Transaction{
					// promoted
					newTx(addr1, 1, 1),
					newTx(addr1, 2, 1),
					newTx(addr1, 3, 1),
					// enqueued
					newTx(addr1, 5, 1),
					newTx(addr1, 8, 1),
					newTx(addr1, 9, 1),
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
				txs: []*types.Transaction{
					// promoted
					newTx(addr1, 2, 1),
					newTx(addr1, 3, 1),
					newTx(addr1, 4, 1),
					newTx(addr1, 5, 1),
					// enqueued
					newTx(addr1, 8, 1),
					newTx(addr1, 9, 1),
					newTx(addr1, 10, 1),
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
			test := test
			t.Run(test.name, func(t *testing.T) {
				t.Parallel()

				pool, err := newTestPool()
				assert.NoError(t, err)
				pool.SetSigner(&mockSigner{})

				// setup prestate
				acc := pool.getOrCreateAccount(addr1)
				acc.setNonce(test.txs[0].Nonce)

				err = pool.addTx(local, test.txs[0])
				assert.NoError(t, err)

				// save the promotion
				req := <-pool.promoteReqCh

				// enqueue remaining
				for i, tx := range test.txs {
					if i == 0 {
						// first was handled
						continue
					}

					err := pool.addTx(local, tx)
					assert.NoError(t, err)
				}

				assert.Equal(t, len(test.txs), len(acc.nonceToTx.mapping))

				pool.handlePromoteRequest(req)

				if test.signal {
					go pool.resetAccounts(map[types.Address]uint64{
						addr1: test.newNonce,
					})
					pool.handlePromoteRequest(<-pool.promoteReqCh)
				} else {
					pool.resetAccounts(map[types.Address]uint64{
						addr1: test.newNonce,
					})
				}

				acc.nonceToTx.lock()

				assert.Equal(t, int(test.expected.accounts[addr1].enqueued+test.expected.accounts[addr1].promoted), len(acc.nonceToTx.mapping))

				acc.nonceToTx.unlock()

				assert.Equal(t, test.expected.slots, pool.gauge.read())
				assert.Equal(t, // enqueued
					test.expected.accounts[addr1].enqueued,
					pool.accounts.get(addr1).enqueued.length())
				assert.Equal(t, // promoted
					test.expected.accounts[addr1].promoted,
					pool.accounts.get(addr1).promoted.length())
			})
		}
	})
}

func TestPop(t *testing.T) {
	t.Parallel()

	pool, err := newTestPool()
	assert.NoError(t, err)
	pool.SetSigner(&mockSigner{})

	// send 1 tx and promote it
	tx1 := newTx(addr1, 0, 1)
	err = pool.addTx(local, tx1)
	assert.NoError(t, err)

	acc := pool.accounts.get(addr1)

	assert.Equal(t, int(1), len(acc.nonceToTx.mapping))
	assert.Equal(t, tx1, acc.nonceToTx.get(tx1.Nonce))

	pool.handlePromoteRequest(<-pool.promoteReqCh)

	assert.Equal(t, uint64(1), pool.gauge.read())
	assert.Equal(t, uint64(1), pool.accounts.get(addr1).promoted.length())

	// pop the tx
	pool.Prepare()
	tx := pool.Peek()
	pool.Pop(tx)

	assert.Equal(t, tx1, tx)

	assert.Equal(t, uint64(0), pool.gauge.read())
	assert.Equal(t, uint64(0), pool.accounts.get(addr1).promoted.length())

	assert.Equal(t, int(0), len(acc.nonceToTx.mapping))
	assert.Equal(t, (*types.Transaction)(nil), acc.nonceToTx.get(tx1.Nonce))
}

func TestDrop(t *testing.T) {
	t.Parallel()

	pool, err := newTestPool()
	assert.NoError(t, err)
	pool.SetSigner(&mockSigner{})

	// send 1 tx and promote it
	tx1 := newTx(addr1, 0, 1)
	err = pool.addTx(local, tx1)
	assert.NoError(t, err)

	acc := pool.accounts.get(addr1)

	pool.handlePromoteRequest(<-pool.promoteReqCh)

	assert.Equal(t, uint64(1), pool.gauge.read())
	assert.Equal(t, uint64(1), pool.accounts.get(addr1).getNonce())
	assert.Equal(t, uint64(1), pool.accounts.get(addr1).promoted.length())

	// pop the tx
	pool.Prepare()
	tx := pool.Peek()
	pool.Drop(tx)
	assert.Equal(t, tx1, tx)

	assert.Equal(t, uint64(0), pool.gauge.read())
	assert.Equal(t, uint64(0), pool.accounts.get(addr1).getNonce())
	assert.Equal(t, uint64(0), pool.accounts.get(addr1).promoted.length())

	assert.Equal(t, int(0), len(acc.nonceToTx.mapping))
	assert.Equal(t, (*types.Transaction)(nil), acc.nonceToTx.get(tx1.Nonce))
}

func TestDemote(t *testing.T) {
	t.Parallel()

	t.Run("Demote increments counter", func(t *testing.T) {
		t.Parallel()
		// create pool
		pool, err := newTestPool()
		assert.NoError(t, err)
		pool.SetSigner(&mockSigner{})

		// send tx
		err = pool.addTx(local, newTx(addr1, 0, 1))
		assert.NoError(t, err)

		pool.handlePromoteRequest(<-pool.promoteReqCh)

		assert.Equal(t, uint64(1), pool.gauge.read())
		assert.Equal(t, uint64(1), pool.accounts.get(addr1).getNonce())
		assert.Equal(t, uint64(1), pool.accounts.get(addr1).promoted.length())
		assert.Equal(t, uint64(0), pool.accounts.get(addr1).Demotions())

		// call demote
		pool.Prepare()
		tx := pool.Peek()
		pool.Demote(tx)

		assert.Equal(t, uint64(1), pool.gauge.read())
		assert.Equal(t, uint64(1), pool.accounts.get(addr1).getNonce())
		assert.Equal(t, uint64(1), pool.accounts.get(addr1).promoted.length())

		// assert counter was incremented
		assert.Equal(t, uint64(1), pool.accounts.get(addr1).Demotions())
	})

	t.Run("Demote calls Drop", func(t *testing.T) {
		t.Parallel()

		// create pool
		pool, err := newTestPool()
		assert.NoError(t, err)
		pool.SetSigner(&mockSigner{})

		// send tx
		err = pool.addTx(local, newTx(addr1, 0, 1))
		assert.NoError(t, err)

		pool.handlePromoteRequest(<-pool.promoteReqCh)

		assert.Equal(t, uint64(1), pool.gauge.read())
		assert.Equal(t, uint64(1), pool.accounts.get(addr1).getNonce())
		assert.Equal(t, uint64(1), pool.accounts.get(addr1).promoted.length())

		// set counter to max allowed demotions
		pool.accounts.get(addr1).demotions = maxAccountDemotions

		// call demote
		pool.Prepare()
		tx := pool.Peek()
		pool.Demote(tx)

		// account was dropped
		assert.Equal(t, uint64(0), pool.gauge.read())
		assert.Equal(t, uint64(0), pool.accounts.get(addr1).getNonce())
		assert.Equal(t, uint64(0), pool.accounts.get(addr1).promoted.length())

		// demotions are reset to 0
		assert.Equal(t, uint64(0), pool.accounts.get(addr1).Demotions())
	})
}

func Test_updateAccountSkipsCounts(t *testing.T) {
	t.Parallel()

	sendTx := func(
		t *testing.T,
		pool *TxPool,
		tx *types.Transaction,
		shouldPromote bool,
	) {
		t.Helper()

		err := pool.addTx(local, tx)
		assert.NoError(t, err)

		if shouldPromote {
			pool.handlePromoteRequest(<-pool.promoteReqCh)
		}
	}

	checkTxExistence := func(t *testing.T, pool *TxPool, txHash types.Hash, shouldExist bool) {
		t.Helper()

		_, ok := pool.index.get(txHash)

		assert.Equal(t, shouldExist, ok)
	}

	t.Run("should drop the first transaction from promoted queue", func(t *testing.T) {
		t.Parallel()
		// create pool
		pool, err := newTestPool()
		assert.NoError(t, err)

		pool.SetSigner(&mockSigner{})

		tx := newTx(addr1, 0, 1)
		sendTx(t, pool, tx, true)

		accountMap := pool.accounts.get(addr1)

		// make sure the transaction is promoted and skips count is zero
		assert.Zero(t, accountMap.enqueued.length())
		assert.Equal(t, uint64(1), accountMap.promoted.length())
		assert.Zero(t, accountMap.skips)
		assert.Equal(t, slotsRequired(tx), pool.gauge.read())
		checkTxExistence(t, pool, tx.Hash, true)

		// set 9 to skips in order to drop transaction next
		accountMap.skips = 9

		pool.updateAccountSkipsCounts(map[types.Address]uint64{
			// empty
		})

		// make sure the account queue is empty and skips is reset
		assert.Zero(t, accountMap.enqueued.length())
		assert.Zero(t, accountMap.promoted.length())
		assert.Zero(t, accountMap.skips)
		assert.Zero(t, pool.gauge.read())
		checkTxExistence(t, pool, tx.Hash, false)
	})

	t.Run("should drop the first transaction from enqueued queue", func(t *testing.T) {
		t.Parallel()
		// create pool
		pool, err := newTestPool()
		assert.NoError(t, err)

		pool.SetSigner(&mockSigner{})

		tx := newTx(addr1, 1, 1) // set non-zero nonce to prevent the tx from being added
		sendTx(t, pool, tx, false)

		accountMap := pool.accounts.get(addr1)

		// make sure the transaction is promoted and skips count is zero
		assert.NotZero(t, accountMap.enqueued.length())
		assert.Zero(t, accountMap.promoted.length())
		assert.Zero(t, accountMap.skips)
		assert.Equal(t, slotsRequired(tx), pool.gauge.read())
		checkTxExistence(t, pool, tx.Hash, true)

		// set 9 to skips in order to drop transaction next
		accountMap.skips = 9

		pool.updateAccountSkipsCounts(map[types.Address]uint64{
			// empty
		})

		// make sure the account queue is empty and skips is reset
		assert.Zero(t, accountMap.enqueued.length())
		assert.Zero(t, accountMap.promoted.length())
		assert.Zero(t, accountMap.skips)
		assert.Zero(t, pool.gauge.read())
		checkTxExistence(t, pool, tx.Hash, false)
	})

	t.Run("should not drop a transaction", func(t *testing.T) {
		t.Parallel()
		// create pool
		pool, err := newTestPool()
		assert.NoError(t, err)

		pool.SetSigner(&mockSigner{})

		tx := newTx(addr1, 0, 1)
		sendTx(t, pool, tx, true)

		accountMap := pool.accounts.get(addr1)

		// make sure the transaction is promoted and skips count is zero
		assert.Zero(t, accountMap.enqueued.length())
		assert.Equal(t, uint64(1), accountMap.promoted.length())
		assert.Zero(t, accountMap.skips)
		assert.Equal(t, slotsRequired(tx), pool.gauge.read())
		checkTxExistence(t, pool, tx.Hash, true)

		// set 9 to skips in order to drop transaction next
		accountMap.skips = 5

		pool.updateAccountSkipsCounts(map[types.Address]uint64{
			addr1: 1,
		})

		// make sure the account queue is empty and skips is reset
		assert.Zero(t, accountMap.enqueued.length())
		assert.Equal(t, uint64(1), accountMap.promoted.length())
		assert.Equal(t, uint64(0), accountMap.skips)
		assert.Equal(t, slotsRequired(tx), pool.gauge.read())
		checkTxExistence(t, pool, tx.Hash, true)
	})

	t.Run("drop should set nonce to current account nonce from store", func(t *testing.T) {
		t.Parallel()

		const storeNonce = uint64(10)

		// create pool
		store := defaultMockStore{
			DefaultHeader: mockHeader,
			nonce:         storeNonce,
		}

		pool, err := newTestPool(store)
		assert.NoError(t, err)

		pool.SetSigner(&mockSigner{})

		accountMap := pool.accounts.initOnce(addr1, storeNonce)
		accountMap.enqueued.push(&types.Transaction{
			Nonce: storeNonce + 2,
			Hash:  types.StringToHash("0xffa"),
		})
		accountMap.enqueued.push(&types.Transaction{
			Nonce: storeNonce + 4,
			Hash:  types.StringToHash("0xff1"),
		})
		accountMap.promoted.push(&types.Transaction{
			Nonce: storeNonce,
			Hash:  types.StringToHash("0xff2"),
		})
		accountMap.promoted.push(&types.Transaction{
			Nonce: storeNonce + 1,
			Hash:  types.StringToHash("0xff3"),
		})
		accountMap.setNonce(storeNonce + 3)
		accountMap.skips = maxAccountSkips - 1

		pool.updateAccountSkipsCounts(map[types.Address]uint64{})

		// make sure the account queue is empty and skips is reset
		assert.Zero(t, accountMap.enqueued.length())
		assert.Equal(t, uint64(0), accountMap.promoted.length())
		assert.Equal(t, uint64(0), accountMap.skips)
		assert.Equal(t, storeNonce, accountMap.getNonce())
	})
}

func Test_TxPool_validateTx(t *testing.T) {
	t.Parallel()

	signer := crypto.NewEIP155Signer(100, true)
	// Generate a private key and address
	defaultKey, defaultAddr := tests.GenerateKeyAndAddr(t)

	setupPool := func() *TxPool {
		header := mockHeader.Copy()
		header.BaseFee = 1000

		pool, err := newTestPool(NewDefaultMockStore(header))
		if err != nil {
			t.Fatalf("cannot create txpool - err: %v\n", err)
		}

		pool.SetBaseFee(header)
		pool.SetSigner(signer)

		return pool
	}

	signTx := func(transaction *types.Transaction) *types.Transaction {
		signedTx, signErr := signer.SignTx(transaction, defaultKey)
		if signErr != nil {
			t.Fatalf("Unable to sign transaction, %v", signErr)
		}

		return signedTx
	}

	t.Run("tx input larger than the TxPoolMaxInitCodeSize", func(t *testing.T) {
		t.Parallel()
		pool := setupPool()
		pool.forks = chain.AllForksEnabled.Copy()

		input := make([]byte, state.TxPoolMaxInitCodeSize+1)
		_, err := rand.Read(input)
		require.NoError(t, err)

		tx := newTx(defaultAddr, 0, 1)
		tx.To = nil
		tx.Input = input

		assert.ErrorIs(t,
			pool.validateTx(signTx(tx)),
			runtime.ErrMaxCodeSizeExceeded,
		)
	})

	t.Run("tx input the same as TxPoolMaxInitCodeSize", func(t *testing.T) {
		t.Parallel()
		pool := setupPool()
		pool.forks = chain.AllForksEnabled.Copy()

		input := make([]byte, state.TxPoolMaxInitCodeSize)
		_, err := rand.Read(input)
		require.NoError(t, err)

		tx := newTx(defaultAddr, 0, 1)
		tx.To = nil
		tx.Input = input
		tx.GasPrice = new(big.Int).SetUint64(pool.GetBaseFee())

		assert.NoError(t,
			pool.validateTx(signTx(tx)),
			runtime.ErrMaxCodeSizeExceeded,
		)
	})

	t.Run("transaction with eip-1559 fields can pass", func(t *testing.T) {
		t.Parallel()

		pool := setupPool()

		tx := newTx(defaultAddr, 0, 1)
		tx.Type = types.DynamicFeeTx
		tx.GasFeeCap = big.NewInt(1100)
		tx.GasTipCap = big.NewInt(10)

		assert.NoError(t, pool.validateTx(signTx(tx)))
	})

	t.Run("eip-1559 tx (gas fee cap less than base fee)", func(t *testing.T) {
		t.Parallel()

		pool := setupPool()

		tx := newTx(defaultAddr, 0, 1)
		tx.Type = types.DynamicFeeTx
		tx.GasFeeCap = big.NewInt(100)
		tx.GasTipCap = big.NewInt(10)

		assert.ErrorIs(t,
			pool.validateTx(signTx(tx)),
			ErrUnderpriced,
		)
	})

	t.Run("eip-1559 tx (gas fee cap less than tip cap)", func(t *testing.T) {
		t.Parallel()

		pool := setupPool()

		tx := newTx(defaultAddr, 0, 1)
		tx.Type = types.DynamicFeeTx
		tx.GasFeeCap = big.NewInt(10000)
		tx.GasTipCap = big.NewInt(100000)

		assert.ErrorIs(t,
			pool.validateTx(signTx(tx)),
			ErrTipAboveFeeCap,
		)
	})

	t.Run("eip-1559 tx (gas fee cap and/or gas tip cap undefined)", func(t *testing.T) {
		t.Parallel()

		pool := setupPool()

		// undefined gas tip cap
		tx := newTx(defaultAddr, 0, 1)
		tx.Type = types.DynamicFeeTx
		tx.GasFeeCap = big.NewInt(10000)

		signedTx := signTx(tx)
		signedTx.GasTipCap = nil

		assert.ErrorIs(t,
			pool.validateTx(signedTx),
			ErrUnderpriced,
		)

		// undefined gas fee cap
		tx = newTx(defaultAddr, 1, 1)
		tx.Type = types.DynamicFeeTx
		tx.GasTipCap = big.NewInt(1000)
		signedTx = signTx(tx)
		signedTx.GasFeeCap = nil

		assert.ErrorIs(t,
			pool.validateTx(signedTx),
			ErrUnderpriced,
		)
	})

	t.Run("eip-1559 tx (gas fee cap and gas tip cap very high)", func(t *testing.T) {
		t.Parallel()

		bitLength := 512

		pool := setupPool()

		// very high gas fee cap
		tx := newTx(defaultAddr, 0, 1)
		tx.Type = types.DynamicFeeTx
		tx.GasFeeCap = new(big.Int).SetBit(new(big.Int), bitLength, 1)

		assert.ErrorIs(t,
			pool.validateTx(signTx(tx)),
			ErrFeeCapVeryHigh,
		)

		// very high gas tip cap
		tx = newTx(defaultAddr, 1, 1)
		tx.Type = types.DynamicFeeTx
		tx.GasTipCap = new(big.Int).SetBit(new(big.Int), bitLength, 1)

		assert.ErrorIs(t,
			pool.validateTx(signTx(tx)),
			ErrTipVeryHigh,
		)
	})

	t.Run("eip-1559 tx placed without eip-1559 fork enabled", func(t *testing.T) {
		t.Parallel()
		pool := setupPool()
		pool.forks = chain.AllForksEnabled.Copy()
		pool.forks.RemoveFork(chain.London)

		tx := newTx(defaultAddr, 0, 1)
		tx.Type = types.DynamicFeeTx
		tx.GasFeeCap = big.NewInt(10000)
		tx.GasTipCap = big.NewInt(100000)

		assert.ErrorIs(t,
			pool.validateTx(signTx(tx)),
			ErrTxTypeNotSupported,
		)
	})
}

/* "Integrated" tests */

// The following tests ensure that the pool's inner event loop
// is handling requests correctly, meaning that we do not have
// to assume its role (like in previous unit tests) and
// perform dispatching/handling on our own

func waitForEvents(
	ctx context.Context,
	subscription *subscribeResult,
	count int,
) []*proto.TxPoolEvent {
	receivedEvents := make([]*proto.TxPoolEvent, 0)

	completed := false
	for !completed {
		select {
		case <-ctx.Done():
			completed = true
		case event := <-subscription.subscriptionChannel:
			receivedEvents = append(receivedEvents, event)

			if len(receivedEvents) == count {
				completed = true
			}
		}
	}

	return receivedEvents
}

type eoa struct {
	Address    types.Address
	PrivateKey *ecdsa.PrivateKey
}

func (e *eoa) create(t *testing.T) *eoa {
	t.Helper()

	e.PrivateKey, e.Address = tests.GenerateKeyAndAddr(t)

	return e
}

func (e *eoa) signTx(t *testing.T, tx *types.Transaction, signer crypto.TxSigner) *types.Transaction {
	t.Helper()

	signedTx, err := signer.SignTx(tx, e.PrivateKey)
	require.NoError(t, err)

	return signedTx
}

var signerEIP155 = crypto.NewEIP155Signer(100, true)

func TestResetAccounts_Promoted(t *testing.T) {
	t.Parallel()

	var (
		eoa1 = new(eoa).create(t)
		eoa2 = new(eoa).create(t)
		eoa3 = new(eoa).create(t)
		eoa4 = new(eoa).create(t)

		addr1 = eoa1.Address
		addr2 = eoa2.Address
		addr3 = eoa3.Address
		addr4 = eoa4.Address
	)

	allTxs :=
		map[types.Address][]*types.Transaction{
			addr1: {
				eoa1.signTx(t, newTx(addr1, 0, 1), signerEIP155), // will be pruned
				eoa1.signTx(t, newTx(addr1, 1, 1), signerEIP155), // will be pruned
				eoa1.signTx(t, newTx(addr1, 2, 1), signerEIP155), // will be pruned
				eoa1.signTx(t, newTx(addr1, 3, 1), signerEIP155), // will be pruned
			},

			addr2: {
				eoa2.signTx(t, newTx(addr2, 0, 1), signerEIP155), // will be pruned
				eoa2.signTx(t, newTx(addr2, 1, 1), signerEIP155), // will be pruned
			},

			addr3: {
				eoa3.signTx(t, newTx(addr3, 0, 1), signerEIP155), // will be pruned
				eoa3.signTx(t, newTx(addr3, 1, 1), signerEIP155), // will be pruned
				eoa3.signTx(t, newTx(addr3, 2, 1), signerEIP155), // will be pruned
			},

			addr4: {
				// all txs will be pruned
				eoa4.signTx(t, newTx(addr4, 0, 1), signerEIP155), // will be pruned
				eoa4.signTx(t, newTx(addr4, 1, 1), signerEIP155), // will be pruned
				eoa4.signTx(t, newTx(addr4, 2, 1), signerEIP155), // will be pruned
				eoa4.signTx(t, newTx(addr4, 3, 1), signerEIP155), // will be pruned
				eoa4.signTx(t, newTx(addr4, 4, 1), signerEIP155), // will be pruned
			},
		}

	newNonces := map[types.Address]uint64{
		addr1: 2,
		addr2: 1,
		addr3: 0,
		addr4: 5,
	}

	expected := result{
		accounts: map[types.Address]accountState{
			addr1: {promoted: 2},
			addr2: {promoted: 1},
			addr3: {promoted: 3},
			addr4: {promoted: 0},
		},
		slots: 2 + 1 + 3 + 0,
	}

	pool, err := newTestPool()
	assert.NoError(t, err)
	pool.SetSigner(signerEIP155)

	pool.Start()
	defer pool.Close()

	promotedSubscription := pool.eventManager.subscribe(
		[]proto.EventType{
			proto.EventType_PROMOTED,
		},
	)

	// setup prestate
	totalTx := 0

	for _, txs := range allTxs {
		for _, tx := range txs {
			totalTx++

			assert.NoError(t, pool.addTx(local, tx))
		}
	}

	ctx, cancelFn := context.WithTimeout(context.Background(), time.Second*10)
	defer cancelFn()

	// All txns should get added
	assert.Len(t, waitForEvents(ctx, promotedSubscription, totalTx), totalTx)
	pool.eventManager.cancelSubscription(promotedSubscription.subscriptionID)

	prunedSubscription := pool.eventManager.subscribe(
		[]proto.EventType{
			proto.EventType_PRUNED_PROMOTED,
		})

	pool.resetAccounts(newNonces)

	ctx, cancelFn = context.WithTimeout(context.Background(), time.Second*10)
	defer cancelFn()

	assert.Len(t, waitForEvents(ctx, prunedSubscription, 8), 8)
	pool.eventManager.cancelSubscription(prunedSubscription.subscriptionID)

	assert.Equal(t, expected.slots, pool.gauge.read())

	for addr := range expected.accounts {
		assert.Equal(t, // enqueued
			expected.accounts[addr].enqueued,
			pool.accounts.get(addr).enqueued.length())

		assert.Equal(t, // promoted
			expected.accounts[addr].promoted,
			pool.accounts.get(addr).promoted.length())

		acc := pool.accounts.get(addr)

		acc.nonceToTx.lock()

		assert.Equal(t, int(expected.accounts[addr].enqueued+expected.accounts[addr].promoted), len(acc.nonceToTx.mapping))
		acc.nonceToTx.unlock()
	}
}

func TestResetAccounts_Enqueued(t *testing.T) {
	t.Parallel()

	commonAssert := func(accounts map[types.Address]accountState, pool *TxPool) {
		for addr := range accounts {
			assert.Equal(t, // enqueued
				accounts[addr].enqueued,
				pool.accounts.get(addr).enqueued.length())

			assert.Equal(t, // promoted
				accounts[addr].promoted,
				pool.accounts.get(addr).promoted.length())
		}
	}

	var (
		eoa1 = new(eoa).create(t)
		eoa2 = new(eoa).create(t)
		eoa3 = new(eoa).create(t)

		addr1 = eoa1.Address
		addr2 = eoa2.Address
		addr3 = eoa3.Address
	)

	t.Run("reset will promote", func(t *testing.T) {
		t.Parallel()

		allTxs := map[types.Address][]*types.Transaction{
			addr1: {
				eoa1.signTx(t, newTx(addr1, 3, 1), signerEIP155),
				eoa1.signTx(t, newTx(addr1, 4, 1), signerEIP155),
				eoa1.signTx(t, newTx(addr1, 5, 1), signerEIP155),
			},
			addr2: {
				eoa2.signTx(t, newTx(addr2, 2, 1), signerEIP155),
				eoa2.signTx(t, newTx(addr2, 3, 1), signerEIP155),
				eoa2.signTx(t, newTx(addr2, 4, 1), signerEIP155),
				eoa2.signTx(t, newTx(addr2, 5, 1), signerEIP155),
				eoa2.signTx(t, newTx(addr2, 6, 1), signerEIP155),
				eoa2.signTx(t, newTx(addr2, 7, 1), signerEIP155),
			},
			addr3: {
				eoa3.signTx(t, newTx(addr3, 7, 1), signerEIP155),
				eoa3.signTx(t, newTx(addr3, 8, 1), signerEIP155),
				eoa3.signTx(t, newTx(addr3, 9, 1), signerEIP155),
			},
		}
		newNonces := map[types.Address]uint64{
			addr1: 3,
			addr2: 4,
			addr3: 8,
		}
		expected := result{
			accounts: map[types.Address]accountState{
				addr1: {
					enqueued: 0,
					promoted: 3, // reset will promote
				},
				addr2: {
					enqueued: 0,
					promoted: 4,
				},
				addr3: {
					enqueued: 0,
					promoted: 2, // reset will promote
				},
			},
			slots: 3 + 4 + 2,
		}

		pool, err := newTestPool()
		assert.NoError(t, err)
		pool.SetSigner(signerEIP155)

		pool.Start()
		defer pool.Close()

		enqueuedSubscription := pool.eventManager.subscribe(
			[]proto.EventType{
				proto.EventType_ENQUEUED,
			},
		)

		promotedSubscription := pool.eventManager.subscribe(
			[]proto.EventType{
				proto.EventType_PROMOTED,
			},
		)

		// setup prestate
		totalTx := 0
		expectedPromoted := uint64(0)
		for addr, txs := range allTxs {
			expectedPromoted += expected.accounts[addr].promoted
			for _, tx := range txs {
				totalTx++
				assert.NoError(t, pool.addTx(local, tx))
			}
		}

		ctx, cancelFn := context.WithTimeout(context.Background(), time.Second*10)
		defer cancelFn()

		// All txns should get added
		assert.Len(t, waitForEvents(ctx, enqueuedSubscription, totalTx), totalTx)
		pool.eventManager.cancelSubscription(enqueuedSubscription.subscriptionID)

		pool.resetAccounts(newNonces)

		ctx, cancelFn = context.WithTimeout(context.Background(), time.Second*10)
		defer cancelFn()

		assert.Len(t, waitForEvents(ctx, promotedSubscription, int(expectedPromoted)), int(expectedPromoted))

		assert.Equal(t, expected.slots, pool.gauge.read())
		commonAssert(expected.accounts, pool)

		for addr := range allTxs {
			acc := pool.accounts.get(addr)

			acc.nonceToTx.lock()
			assert.Equal(t, int(expected.accounts[addr].enqueued+expected.accounts[addr].promoted), len(acc.nonceToTx.mapping))
			acc.nonceToTx.unlock()
		}
	})

	t.Run("reset will not promote", func(t *testing.T) {
		t.Parallel()

		allTxs := map[types.Address][]*types.Transaction{
			addr1: {
				newTx(addr1, 1, 1),
				newTx(addr1, 2, 1),
				newTx(addr1, 3, 1),
				newTx(addr1, 4, 1),
			},
			addr2: {
				newTx(addr2, 3, 3),
				newTx(addr2, 4, 3),
				newTx(addr2, 5, 3),
				newTx(addr2, 6, 3),
			},
			addr3: {
				newTx(addr3, 7, 1),
				newTx(addr3, 8, 1),
				newTx(addr3, 9, 1),
			},
		}
		newNonces := map[types.Address]uint64{
			addr1: 5,
			addr2: 7,
			addr3: 12,
		}
		expected := result{
			accounts: map[types.Address]accountState{
				addr1: {
					enqueued: 0,
					promoted: 0, // reset will promote
				},
				addr2: {
					enqueued: 0,
					promoted: 0,
				},
				addr3: {
					enqueued: 0,
					promoted: 0, // reset will promote
				},
			},
			slots: 0 + 0 + 0,
		}

		pool, err := newTestPool()
		assert.NoError(t, err)
		pool.SetSigner(&mockSigner{})

		pool.Start()
		defer pool.Close()

		enqueuedSubscription := pool.eventManager.subscribe(
			[]proto.EventType{
				proto.EventType_ENQUEUED,
			},
		)

		// setup prestate
		expectedEnqueuedTx := 0
		expectedPromotedTx := uint64(0)
		for addr, txs := range allTxs {
			expectedPromotedTx += expected.accounts[addr].promoted
			for _, tx := range txs {
				expectedEnqueuedTx++
				assert.NoError(t, pool.addTx(local, tx))
			}
		}

		ctx, cancelFn := context.WithTimeout(context.Background(), time.Second*10)
		defer cancelFn()

		// All txns should get added
		assert.Len(t, waitForEvents(ctx, enqueuedSubscription, expectedEnqueuedTx), expectedEnqueuedTx)
		pool.eventManager.cancelSubscription(enqueuedSubscription.subscriptionID)

		pool.resetAccounts(newNonces)

		assert.Equal(t, expected.slots, pool.gauge.read())
		commonAssert(expected.accounts, pool)

		for addr := range allTxs {
			acc := pool.accounts.get(addr)

			acc.nonceToTx.lock()
			assert.Equal(t, int(expected.accounts[addr].enqueued+expected.accounts[addr].promoted), len(acc.nonceToTx.mapping))
			acc.nonceToTx.unlock()
		}
	})
}

func TestExecutablesOrder(t *testing.T) {
	t.Parallel()

	newPricedTx := func(
		addr types.Address, nonce, gasPrice uint64, gasFeeCap uint64, value uint64) *types.Transaction {
		tx := newTx(addr, nonce, 1)
		tx.Value = new(big.Int).SetUint64(value)

		if gasPrice == 0 {
			tx.Type = types.DynamicFeeTx
			tx.GasFeeCap = new(big.Int).SetUint64(gasFeeCap)
			tx.GasTipCap = new(big.Int).SetUint64(2)
			tx.GasPrice = big.NewInt(0)
		} else {
			tx.Type = types.LegacyTx
			tx.GasPrice = new(big.Int).SetUint64(gasPrice)
		}

		return tx
	}

	testCases := []struct {
		name               string
		allTxs             map[types.Address][]*types.Transaction
		expectedPriceOrder [][2]uint64
	}{
		{
			name: "case #1",
			allTxs: map[types.Address][]*types.Transaction{
				addr1: {
					newPricedTx(addr1, 0, 1, 0, 400),
				},
				addr2: {
					newPricedTx(addr2, 0, 2, 0, 500),
				},
				addr3: {
					newPricedTx(addr3, 0, 3, 0, 200),
				},
				addr4: {
					newPricedTx(addr4, 0, 4, 0, 300),
				},
				addr5: {
					newPricedTx(addr5, 0, 5, 0, 100),
				},
			},
			expectedPriceOrder: [][2]uint64{
				{5, 100},
				{4, 300},
				{3, 200},
				{2, 500},
				{1, 400},
			},
		},
		{
			name: "case #2",
			allTxs: map[types.Address][]*types.Transaction{
				addr1: {
					newPricedTx(addr1, 1, 3, 0, 200),
					newPricedTx(addr1, 2, 3, 0, 500),
					newPricedTx(addr1, 0, 3, 0, 300),
				},
				addr2: {
					newPricedTx(addr2, 0, 2, 0, 700),
					newPricedTx(addr2, 1, 2, 0, 900),
					newPricedTx(addr2, 2, 2, 0, 800),
				},
				addr3: {
					newPricedTx(addr3, 2, 1, 0, 100),
					newPricedTx(addr3, 1, 1, 0, 50),
					newPricedTx(addr3, 0, 1, 0, 75),
				},
			},
			expectedPriceOrder: [][2]uint64{
				{3, 300},
				{3, 200},
				{3, 500},
				{2, 700},
				{2, 900},
				{2, 800},
				{1, 75},
				{1, 50},
				{1, 100},
			},
		},
		{
			name: "case #3",
			allTxs: map[types.Address][]*types.Transaction{
				addr1: {
					newPricedTx(addr1, 1, 5, 0, 100),
					newPricedTx(addr1, 0, 9, 0, 100),
					newPricedTx(addr1, 2, 3, 0, 100),
				},
				addr2: {
					newPricedTx(addr2, 0, 9, 0, 100),
					newPricedTx(addr2, 1, 3, 0, 100),
					newPricedTx(addr2, 2, 1, 0, 100),
				},
			},
			expectedPriceOrder: [][2]uint64{
				{9, 100},
				{9, 100},
				{5, 100},
				{3, 100},
				{3, 100},
				{1, 100},
			},
		},
		{
			name: "case #4",
			allTxs: map[types.Address][]*types.Transaction{
				addr1: {
					newPricedTx(addr1, 0, 0, 70, 1),
					newPricedTx(addr1, 1, 0, 50, 2),
					newPricedTx(addr1, 2, 0, 20, 3),
				},
				addr2: {
					newPricedTx(addr2, 0, 0, 90, 4),
					newPricedTx(addr2, 1, 0, 80, 5),
					newPricedTx(addr2, 2, 0, 10, 6),
				},
				addr3: {
					newPricedTx(addr3, 0, 0, 100, 7),
					newPricedTx(addr3, 1, 0, 60, 8),
					newPricedTx(addr3, 2, 0, 40, 9),
				},
			},
			expectedPriceOrder: [][2]uint64{
				{0, 7},
				{0, 4},
				{0, 5},
				{0, 1},
				{0, 8},
				{0, 2},
				{0, 9},
				{0, 3},
				{0, 6},
			},
		},
	}

	for _, test := range testCases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			pool, err := newTestPool()
			assert.NoError(t, err)

			pool.baseFee = defaultBaseFee
			pool.SetSigner(&mockSigner{})

			pool.Start()
			defer pool.Close()

			subscription := pool.eventManager.subscribe(
				[]proto.EventType{proto.EventType_PROMOTED},
			)

			expectedPromotedTx := 0
			for _, txs := range test.allTxs {
				for _, tx := range txs {
					expectedPromotedTx++
					// send all txs
					assert.NoError(t, pool.addTx(local, tx))
				}
			}

			ctx, cancelFn := context.WithTimeout(context.Background(), time.Second*10)
			defer cancelFn()

			// All txns should get added
			require.Len(t, waitForEvents(ctx, subscription, expectedPromotedTx), expectedPromotedTx)
			require.Equal(t, uint64(len(test.expectedPriceOrder)), pool.accounts.promoted())

			pool.Prepare()

			var successful []*types.Transaction
			for {
				tx := pool.Peek()
				if tx == nil {
					break
				}

				pool.Pop(tx)
				successful = append(successful, tx)
			}

			require.Len(t, successful, expectedPromotedTx)

			// verify the highest priced transactions
			// were processed first
			for i, tx := range successful {
				require.Equal(t, test.expectedPriceOrder[i][0], tx.GasPrice.Uint64())
				require.Equal(t, test.expectedPriceOrder[i][1], tx.Value.Uint64())
			}
		})
	}
}

type status int

// Status of a transaction resulted
// from a transition write attempt
const (
	// if a tx is recoverable,
	// account is excluded from
	// further processing
	recoverable status = iota

	// if a tx is unrecoverable,
	// entire account is dropped
	unrecoverable

	ok
)

type statusTx struct {
	tx     *types.Transaction
	status status
}

func TestRecovery(t *testing.T) {
	t.Parallel()

	commonAssert := func(accounts map[types.Address]accountState, pool *TxPool) {
		for addr := range accounts {
			assert.Equal(t, // nextNonce
				accounts[addr].nextNonce,
				pool.accounts.get(addr).getNonce())

			assert.Equal(t, // enqueued
				accounts[addr].enqueued,
				pool.accounts.get(addr).enqueued.length())

			assert.Equal(t, // promoted
				accounts[addr].promoted,
				pool.accounts.get(addr).promoted.length())
		}
	}

	testCases := []struct {
		name     string
		allTxs   map[types.Address][]statusTx
		expected result
	}{
		{
			name: "unrecoverable drops account",
			allTxs: map[types.Address][]statusTx{
				addr1: {
					{newTx(addr1, 0, 1), ok},
					{newTx(addr1, 1, 1), unrecoverable},
					{newTx(addr1, 2, 1), recoverable},
					{newTx(addr1, 3, 1), recoverable},
					{newTx(addr1, 4, 1), recoverable},
				},

				addr2: {
					{newTx(addr2, 9, 1), unrecoverable},
					{newTx(addr2, 10, 1), recoverable},
				},

				addr3: {
					{newTx(addr3, 5, 1), ok},
					{newTx(addr3, 6, 1), recoverable},
					{newTx(addr3, 7, 1), recoverable},
					{newTx(addr3, 8, 1), recoverable},
				},
			},
			expected: result{
				slots: 3, // addr3
				accounts: map[types.Address]accountState{
					addr1: {
						enqueued:  0,
						promoted:  0,
						nextNonce: 1,
					},

					addr2: {
						enqueued:  0,
						promoted:  0,
						nextNonce: 9,
					},

					addr3: {
						enqueued:  0,
						promoted:  3,
						nextNonce: 9,
					},
				},
			},
		},
		{
			name: "recoverable remains in account",
			allTxs: map[types.Address][]statusTx{
				addr1: {
					{newTx(addr1, 0, 1), ok},
					{newTx(addr1, 1, 1), ok},
					{newTx(addr1, 2, 1), ok},
					{newTx(addr1, 3, 1), recoverable},
					{newTx(addr1, 4, 1), recoverable},
				},
				addr2: {
					{newTx(addr2, 9, 1), recoverable},
					{newTx(addr2, 10, 1), recoverable},
				},
			},
			expected: result{
				slots: 4,
				accounts: map[types.Address]accountState{
					addr1: {
						enqueued:  0,
						promoted:  2,
						nextNonce: 5,
					},
					addr2: {
						enqueued:  0,
						promoted:  2,
						nextNonce: 11,
					},
				},
			},
		},
	}

	for _, test := range testCases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

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
			pool.SetSigner(&mockSigner{})

			pool.Start()
			defer pool.Close()

			promoteSubscription := pool.eventManager.subscribe(
				[]proto.EventType{proto.EventType_PROMOTED},
			)

			// setup prestate
			totalTx := 0
			expectedEnqueued := uint64(0)
			for addr, txs := range test.allTxs {
				// preset nonce so promotions can happen
				acc := pool.getOrCreateAccount(addr)
				acc.setNonce(txs[0].tx.Nonce)

				expectedEnqueued += test.expected.accounts[addr].enqueued

				// send txs
				for _, sTx := range txs {
					totalTx++
					assert.NoError(t, pool.addTx(local, sTx.tx))
				}
			}

			ctx, cancelFn := context.WithTimeout(context.Background(), time.Second*10)
			defer cancelFn()

			// All txns should get added
			assert.Len(t, waitForEvents(ctx, promoteSubscription, totalTx), totalTx)

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

			assert.Equal(t, test.expected.slots, pool.gauge.read())
			commonAssert(test.expected.accounts, pool)
		})
	}
}

func TestGetTxs(t *testing.T) {
	t.Parallel()

	var (
		eoa1 = new(eoa).create(t)
		eoa2 = new(eoa).create(t)
		eoa3 = new(eoa).create(t)

		addr1 = eoa1.Address
		addr2 = eoa2.Address
		addr3 = eoa3.Address
	)

	testCases := []struct {
		name             string
		allTxs           map[types.Address][]*types.Transaction
		expectedEnqueued map[types.Address][]*types.Transaction
		expectedPromoted map[types.Address][]*types.Transaction
	}{
		{
			name: "get promoted txs",
			allTxs: map[types.Address][]*types.Transaction{
				addr1: {
					eoa1.signTx(t, newTx(addr1, 0, 1), signerEIP155),
					eoa1.signTx(t, newTx(addr1, 1, 1), signerEIP155),
					eoa1.signTx(t, newTx(addr1, 2, 1), signerEIP155),
				},

				addr2: {
					eoa2.signTx(t, newTx(addr2, 0, 1), signerEIP155),
					eoa2.signTx(t, newTx(addr2, 1, 1), signerEIP155),
					eoa2.signTx(t, newTx(addr2, 2, 1), signerEIP155),
				},

				addr3: {
					eoa3.signTx(t, newTx(addr3, 0, 1), signerEIP155),
					eoa3.signTx(t, newTx(addr3, 1, 1), signerEIP155),
					eoa3.signTx(t, newTx(addr3, 2, 1), signerEIP155),
				},
			},
			expectedPromoted: map[types.Address][]*types.Transaction{
				addr1: {
					eoa1.signTx(t, newTx(addr1, 0, 1), signerEIP155),
					eoa1.signTx(t, newTx(addr1, 1, 1), signerEIP155),
					eoa1.signTx(t, newTx(addr1, 2, 1), signerEIP155),
				},

				addr2: {
					eoa2.signTx(t, newTx(addr2, 0, 1), signerEIP155),
					eoa2.signTx(t, newTx(addr2, 1, 1), signerEIP155),
					eoa2.signTx(t, newTx(addr2, 2, 1), signerEIP155),
				},

				addr3: {
					eoa3.signTx(t, newTx(addr3, 0, 1), signerEIP155),
					eoa3.signTx(t, newTx(addr3, 1, 1), signerEIP155),
					eoa3.signTx(t, newTx(addr3, 2, 1), signerEIP155),
				},
			},
		},
		{
			name: "get all txs",
			allTxs: map[types.Address][]*types.Transaction{
				addr1: {
					eoa1.signTx(t, newTx(addr1, 0, 1), signerEIP155),
					eoa1.signTx(t, newTx(addr1, 1, 1), signerEIP155),
					eoa1.signTx(t, newTx(addr1, 2, 1), signerEIP155),
					// enqueued
					eoa1.signTx(t, newTx(addr1, 10, 1), signerEIP155),
					eoa1.signTx(t, newTx(addr1, 11, 1), signerEIP155),
					eoa1.signTx(t, newTx(addr1, 12, 1), signerEIP155),
				},

				addr2: {
					eoa2.signTx(t, newTx(addr2, 0, 1), signerEIP155),
					eoa2.signTx(t, newTx(addr2, 1, 1), signerEIP155),
					eoa2.signTx(t, newTx(addr2, 2, 1), signerEIP155),

					// enqueued
					eoa2.signTx(t, newTx(addr2, 10, 1), signerEIP155),
					eoa2.signTx(t, newTx(addr2, 11, 1), signerEIP155),
					eoa2.signTx(t, newTx(addr2, 12, 1), signerEIP155),
				},

				addr3: {
					eoa3.signTx(t, newTx(addr3, 0, 1), signerEIP155),
					eoa3.signTx(t, newTx(addr3, 1, 1), signerEIP155),
					eoa3.signTx(t, newTx(addr3, 2, 1), signerEIP155),

					// enqueued
					eoa3.signTx(t, newTx(addr3, 10, 1), signerEIP155),
					eoa3.signTx(t, newTx(addr3, 11, 1), signerEIP155),
					eoa3.signTx(t, newTx(addr3, 12, 1), signerEIP155),
				},
			},
			expectedPromoted: map[types.Address][]*types.Transaction{
				addr1: {
					eoa1.signTx(t, newTx(addr1, 0, 1), signerEIP155),
					eoa1.signTx(t, newTx(addr1, 1, 1), signerEIP155),
					eoa1.signTx(t, newTx(addr1, 2, 1), signerEIP155),
				},

				addr2: {
					eoa2.signTx(t, newTx(addr2, 0, 1), signerEIP155),
					eoa2.signTx(t, newTx(addr2, 1, 1), signerEIP155),
					eoa2.signTx(t, newTx(addr2, 2, 1), signerEIP155),
				},

				addr3: {
					eoa3.signTx(t, newTx(addr3, 0, 1), signerEIP155),
					eoa3.signTx(t, newTx(addr3, 1, 1), signerEIP155),
					eoa3.signTx(t, newTx(addr3, 2, 1), signerEIP155),
				},
			},
			expectedEnqueued: map[types.Address][]*types.Transaction{
				addr1: {
					eoa1.signTx(t, newTx(addr1, 10, 1), signerEIP155),
					eoa1.signTx(t, newTx(addr1, 11, 1), signerEIP155),
					eoa1.signTx(t, newTx(addr1, 12, 1), signerEIP155),
				},

				addr2: {
					eoa2.signTx(t, newTx(addr2, 10, 1), signerEIP155),
					eoa2.signTx(t, newTx(addr2, 11, 1), signerEIP155),
					eoa2.signTx(t, newTx(addr2, 12, 1), signerEIP155),
				},

				addr3: {
					eoa3.signTx(t, newTx(addr3, 10, 1), signerEIP155),
					eoa3.signTx(t, newTx(addr3, 11, 1), signerEIP155),
					eoa3.signTx(t, newTx(addr3, 12, 1), signerEIP155),
				},
			},
		},
	}

	for _, test := range testCases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			find := func(
				tx *types.Transaction,
				all map[types.Address][]*types.Transaction,
			) bool {
				for _, txx := range all[tx.From] {
					if tx.Nonce == txx.Nonce {
						return true
					}
				}

				return false
			}

			pool, err := newTestPool()
			assert.NoError(t, err)
			pool.SetSigner(signerEIP155)

			pool.Start()
			defer pool.Close()

			promoteSubscription := pool.eventManager.subscribe(
				[]proto.EventType{
					proto.EventType_PROMOTED,
				},
			)

			enqueueSubscription := pool.eventManager.subscribe(
				[]proto.EventType{
					proto.EventType_ENQUEUED,
				},
			)

			// send txs
			expectedPromotedTx := 0
			for _, txs := range test.allTxs {
				nonce := uint64(0)
				promotable := uint64(0)
				for _, tx := range txs {
					// send all txs
					if tx.Nonce == nonce+promotable {
						promotable++
					}

					assert.NoError(t, pool.addTx(local, tx))
				}

				expectedPromotedTx += int(promotable)
			}

			ctx, cancelFn := context.WithTimeout(context.Background(), time.Second*10)
			defer cancelFn()

			// Wait for promoted transactions
			assert.Len(t, waitForEvents(ctx, promoteSubscription, expectedPromotedTx), expectedPromotedTx)

			// Wait for enqueued transactions, if any are present
			expectedEnqueuedTx := expectedPromotedTx - len(test.allTxs)

			if expectedEnqueuedTx > 0 {
				ctx, cancelFn = context.WithTimeout(context.Background(), time.Second*10)
				defer cancelFn()

				assert.Len(t, waitForEvents(ctx, enqueueSubscription, expectedEnqueuedTx), expectedEnqueuedTx)
			}

			allPromoted, allEnqueued := pool.GetTxs(true)

			// assert promoted
			for _, txs := range allPromoted {
				for _, tx := range txs {
					found := find(tx, test.expectedPromoted)
					assert.True(t, found)
				}
			}

			// assert enqueued
			for _, txs := range allEnqueued {
				for _, tx := range txs {
					found := find(tx, test.expectedEnqueued)
					assert.True(t, found)
				}
			}
		})
	}
}

func TestSetSealing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		// the value that is set before the operation
		initialValue bool
		// the value to be set by SetSealing
		value bool
		// the value that is set after the operation
		expectedValue bool
	}{
		{
			initialValue:  false,
			value:         false,
			expectedValue: false,
		},
		{
			initialValue:  false,
			value:         true,
			expectedValue: true,
		},
		{
			initialValue:  true,
			value:         false,
			expectedValue: false,
		},
		{
			initialValue:  true,
			value:         true,
			expectedValue: true,
		},
	}

	for _, test := range tests {
		test := test
		name := fmt.Sprintf("initial=%t, value=%t", test.initialValue, test.value)

		t.Run(name, func(t *testing.T) {
			t.Parallel()

			pool, err := newTestPool()
			assert.NoError(t, err)

			// Set initial value
			pool.sealing.Store(false)
			if test.initialValue {
				pool.sealing.Store(true)
			}

			// call the target
			pool.SetSealing(test.value)

			// get the result
			assert.Equal(
				t,
				test.expectedValue,
				pool.sealing.Load(),
			)
		})
	}
}

func TestBatchTx_SingleAccount(t *testing.T) {
	t.Parallel()

	_, addr := tests.GenerateKeyAndAddr(t)

	pool, err := newTestPool()
	assert.NoError(t, err)

	pool.SetSigner(&mockSigner{})

	// start event handler goroutines
	pool.Start()
	defer pool.Close()

	// subscribe to enqueue and promote events
	subscription := pool.eventManager.subscribe([]proto.EventType{proto.EventType_ENQUEUED, proto.EventType_PROMOTED})

	txHashMap := map[types.Hash]struct{}{}
	// mutex for txHashMap
	mux := &sync.RWMutex{}
	counter := uint64(0)

	// run max number of addTx concurrently
	for i := 0; i < int(defaultMaxAccountEnqueued); i++ {
		go func(i uint64) {
			tx := newTx(addr, i, 1)

			tx.ComputeHash(1)

			// add transaction hash to map
			mux.Lock()
			txHashMap[tx.Hash] = struct{}{}
			mux.Unlock()

			// submit transaction to pool
			assert.NoError(t, pool.addTx(local, tx))

			atomic.AddUint64(&counter, 1)
		}(uint64(i))
	}

	enqueuedCount := 0
	promotedCount := 0
	ev := (*proto.TxPoolEvent)(nil)

	// wait for all the submitted transactions to be promoted
	for {
		select {
		case ev = <-subscription.subscriptionChannel:
		case <-time.After(time.Second * 3):
			t.Fatal(fmt.Sprintf("timeout. processed: %d/%d and %d/%d. Added: %d",
				enqueuedCount, defaultMaxAccountEnqueued, promotedCount, defaultMaxAccountEnqueued,
				atomic.LoadUint64(&counter)))
		}

		// check if valid transaction hash
		mux.Lock()
		_, hashExists := txHashMap[types.StringToHash(ev.TxHash)]
		mux.Unlock()

		assert.True(t, hashExists)

		// increment corresponding event type's count
		if ev.Type == proto.EventType_ENQUEUED {
			enqueuedCount++
		} else if ev.Type == proto.EventType_PROMOTED {
			promotedCount++
		}

		if enqueuedCount == int(defaultMaxAccountEnqueued) && promotedCount == int(defaultMaxAccountEnqueued) {
			// compare local tracker to pool internal
			assert.Equal(t, defaultMaxAccountEnqueued, pool.Length())

			// all transactions are promoted
			break
		}
	}

	acc := pool.accounts.get(addr)

	acc.nonceToTx.lock()

	assert.Equal(t, int(defaultMaxAccountEnqueued), len(acc.nonceToTx.mapping))

	acc.nonceToTx.unlock()
}

func TestAddTxsInOrder(t *testing.T) {
	t.Parallel()

	const accountCount = 10

	type container struct {
		key  *ecdsa.PrivateKey
		addr types.Address
		txs  []*types.Transaction
	}

	addrsTxs := [accountCount]container{}

	for i := 0; i < accountCount; i++ {
		key, err := crypto.GenerateECDSAKey()
		require.NoError(t, err)

		addrsTxs[i] = container{
			key:  key,
			addr: crypto.PubKeyToAddress(&key.PublicKey),
			txs:  make([]*types.Transaction, defaultMaxAccountEnqueued),
		}

		for j := uint64(0); j < defaultMaxAccountEnqueued; j++ {
			addrsTxs[i].txs[j] = newTx(addrsTxs[i].addr, j, uint64(1))
		}
	}

	pool, err := newTestPool()
	require.NoError(t, err)

	signer := crypto.NewEIP155Signer(100, true)

	pool.SetSigner(signer)
	pool.Start()

	wg := new(sync.WaitGroup)
	wg.Add(len(addrsTxs) * int(defaultMaxAccountEnqueued))

	for _, atx := range addrsTxs {
		for i, tx := range atx.txs {
			go func(i int, tx *types.Transaction, key *ecdsa.PrivateKey) {
				if i%2 == 1 {
					time.Sleep(time.Millisecond * 50)
				}

				signedTx, err := signer.SignTx(tx, key)
				require.NoError(t, err)

				require.NoError(t, pool.addTx(local, signedTx))

				wg.Done()
			}(i, tx, atx.key)
		}
	}

	wg.Wait()

	time.Sleep(100 * time.Millisecond)

	pool.Close()

	for _, addrtx := range addrsTxs {
		acc := pool.accounts.get(addrtx.addr)
		require.NotNil(t, acc)

		acc.enqueued.lock(false)
		enqueuedCount := acc.enqueued.length()
		acc.enqueued.unlock()

		acc.promoted.lock(false)
		promotedCount := acc.promoted.length()
		acc.promoted.unlock()

		assert.Equal(t, uint64(0), enqueuedCount)
		assert.Len(t, acc.nonceToTx.mapping, int(promotedCount))
	}
}

func TestResetWithHeadersSetsBaseFee(t *testing.T) {
	t.Parallel()

	blocks := []*types.Block{
		{
			Header: &types.Header{
				BaseFee: 100,
				Hash:    types.Hash{1},
			},
		},
		{
			Header: &types.Header{
				BaseFee: 1000,
				Hash:    types.Hash{1},
			},
		},
		{
			Header: &types.Header{
				BaseFee: 2000,
				Hash:    types.Hash{2},
			},
		},
	}

	store := NewDefaultMockStore(blocks[0].Header)
	store.getBlockByHashFn = func(h types.Hash, b bool) (*types.Block, bool) {
		for _, b := range blocks {
			if b.Header.Hash == h {
				return b, true
			}
		}

		return nil, false
	}

	pool, err := newTestPool(store)
	require.NoError(t, err)

	pool.SetBaseFee(blocks[0].Header)

	pool.ResetWithHeaders()
	assert.Equal(t, blocks[0].Header.BaseFee, pool.GetBaseFee())

	pool.ResetWithHeaders(blocks[len(blocks)-2].Header, blocks[len(blocks)-1].Header)
	assert.Equal(t, blocks[len(blocks)-1].Header.BaseFee, pool.GetBaseFee())
}

func TestAddTx_TxReplacement(t *testing.T) {
	t.Parallel()

	const (
		firstAccountNonce  = 1
		secondAccountNonce = 2
	)

	poolSigner := crypto.NewEIP155Signer(100, true)
	firstKey, firstKeyAddr := tests.GenerateKeyAndAddr(t)
	secondKey, secondKeyAddr := tests.GenerateKeyAndAddr(t)

	createDynamicTx := func(
		t *testing.T, nonce, gasFeeCap, gasTipCap uint64,
		key *ecdsa.PrivateKey, addr types.Address) *types.Transaction {
		t.Helper()

		tx := newTx(addr, nonce, 1)
		tx.Type = types.DynamicFeeTx
		tx.Input = nil
		tx.GasPrice = nil
		tx.GasTipCap = new(big.Int).SetUint64(gasTipCap)
		tx.GasFeeCap = new(big.Int).SetUint64(gasFeeCap)

		singedTx, err := poolSigner.SignTx(tx, key)
		require.NoError(t, err)

		singedTx.ComputeHash(0)

		return singedTx
	}

	createLegacyTx := func(
		t *testing.T, nonce, gasPrice uint64,
		key *ecdsa.PrivateKey, addr types.Address) *types.Transaction {
		t.Helper()

		tx := newTx(addr, nonce, 1)
		tx.Input = nil
		tx.GasPrice = new(big.Int).SetUint64(gasPrice)

		singedTx, err := poolSigner.SignTx(tx, key)
		require.NoError(t, err)

		singedTx.ComputeHash(0)

		return singedTx
	}

	pool, err := newTestPool()
	require.NoError(t, err)

	pool.baseFee = 100
	pool.SetSigner(poolSigner)

	txLegacy := createLegacyTx(t, firstAccountNonce, 100, firstKey, firstKeyAddr)
	txDynamic := createDynamicTx(t, secondAccountNonce, 120, 120, secondKey, secondKeyAddr)

	ac1 := pool.accounts.initOnce(firstKeyAddr, firstAccountNonce)
	ac2 := pool.accounts.initOnce(secondKeyAddr, secondAccountNonce)

	ac1.nonceToTx.mapping[firstAccountNonce] = txLegacy
	ac2.nonceToTx.mapping[secondAccountNonce] = txDynamic

	ac1.enqueued = newAccountQueue()
	ac2.enqueued = newAccountQueue()
	ac1.enqueued.queue = minNonceQueue{txLegacy}
	ac2.enqueued.queue = minNonceQueue{txDynamic}

	// These tests can not be executed in parallel because Success test change state

	// ErrAlreadyKnown
	tx1 := createLegacyTx(t,
		firstAccountNonce, txLegacy.GasPrice.Uint64(), firstKey, firstKeyAddr)
	tx2 := createDynamicTx(t,
		secondAccountNonce, txDynamic.GasFeeCap.Uint64(), txDynamic.GasTipCap.Uint64(), secondKey, secondKeyAddr)

	assert.ErrorIs(t, pool.addTx(local, tx1), ErrAlreadyKnown)
	assert.ErrorIs(t, pool.addTx(local, tx2), ErrAlreadyKnown)

	// ErrUnderpriced
	tx1 = createLegacyTx(t,
		firstAccountNonce, 90, firstKey, firstKeyAddr)
	tx2 = createDynamicTx(t,
		secondAccountNonce, 90, 90, secondKey, secondKeyAddr)
	tx3 := createLegacyTx(t,
		secondAccountNonce, 110, secondKey, secondKeyAddr)
	tx4 := createDynamicTx(t,
		firstAccountNonce, 90, 90, firstKey, firstKeyAddr)

	assert.ErrorIs(t, pool.addTx(local, tx1), ErrUnderpriced)
	assert.ErrorIs(t, pool.addTx(local, tx2), ErrUnderpriced)
	assert.ErrorIs(t, pool.addTx(local, tx3), ErrReplacementUnderpriced)
	assert.ErrorIs(t, pool.addTx(local, tx4), ErrUnderpriced)

	// Success
	tx1 = createLegacyTx(t,
		secondAccountNonce, 180, secondKey, secondKeyAddr)
	tx2 = createDynamicTx(t,
		firstAccountNonce, 200, 200, firstKey, firstKeyAddr)

	require.NoError(t, pool.addTx(local, tx1))
	require.NoError(t, pool.addTx(local, tx2))

	assert.Equal(t, ac1.nonceToTx.mapping[firstAccountNonce], tx2)
	assert.Equal(t, ac2.nonceToTx.mapping[secondAccountNonce], tx1)
	assert.Equal(t, ac1.enqueued.queue[0], tx2)
	assert.Equal(t, ac2.enqueued.queue[0], tx1)
}

// getDefaultEnabledForks returns hardcoded set of forks
// that are enabled by default from the genesis block
func getDefaultEnabledForks() *chain.Forks {
	return &chain.Forks{
		chain.Homestead: chain.NewFork(0),
		chain.Istanbul:  chain.NewFork(0),
		chain.London:    chain.NewFork(0),
	}
}

func BenchmarkAddTxTime(b *testing.B) {
	b.Run("benchmark add one tx", func(b *testing.B) {
		signer := crypto.NewEIP155Signer(100, true)

		key, err := crypto.GenerateECDSAKey()
		if err != nil {
			b.Fatal(err)
		}

		signedTx, err := signer.SignTx(newTx(crypto.PubKeyToAddress(&key.PublicKey), 0, 1), key)
		if err != nil {
			b.Fatal(err)
		}

		for i := 0; i < b.N; i++ {
			pool, err := newTestPool()
			if err != nil {
				b.Fatal("fail to create pool", "err", err)
			}

			pool.SetSigner(signer)

			err = pool.addTx(local, signedTx)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("benchmark fill account", func(b *testing.B) {
		signer := crypto.NewEIP155Signer(100, true)

		key, err := crypto.GenerateECDSAKey()
		if err != nil {
			b.Fatal(err)
		}

		addr := crypto.PubKeyToAddress(&key.PublicKey)
		txs := make([]*types.Transaction, defaultMaxAccountEnqueued)

		for i := range txs {
			txs[i], err = signer.SignTx(newTx(addr, uint64(i), uint64(1)), key)
			if err != nil {
				b.Fatal(err)
			}
		}

		for i := 0; i < b.N; i++ {
			pool, err := newTestPool()
			if err != nil {
				b.Fatal("fail to create pool", "err", err)
			}

			pool.SetSigner(signer)

			for i := 0; i < len(txs); i++ {
				err = pool.addTx(local, txs[i])
				if err != nil {
					b.Fatal(err)
				}
			}
		}
	})
}
