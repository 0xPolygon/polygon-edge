package txpool

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"fmt"
	"math/big"
	"strconv"
	"testing"

	"github.com/0xPolygon/polygon-sdk/chain"
	"github.com/0xPolygon/polygon-sdk/crypto"
	"github.com/0xPolygon/polygon-sdk/helper/tests"
	"github.com/0xPolygon/polygon-sdk/network"
	"github.com/0xPolygon/polygon-sdk/txpool/proto"
	"github.com/0xPolygon/polygon-sdk/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
)

var forks = &chain.Forks{
	Homestead: chain.NewFork(0),
	Istanbul:  chain.NewFork(0),
}

const (
	defaultPriceLimit uint64 = 1
	defaultMaxSlots   uint64 = 4096
	defaultSpeedUpMin uint64 = 0
)

var (
	addr1 = types.Address{0x1}
	addr2 = types.Address{0x2}
)

type mockStore struct {
}

func (m *mockStore) GetNonce(types.Hash, types.Address) uint64 {
	return 0
}

func (m *mockStore) GetBlockByHash(types.Hash, bool) (*types.Block, bool) {
	return nil, false
}

func (m *mockStore) GetBalance(types.Hash, types.Address) (*big.Int, error) {
	balance, _ := big.NewInt(0).SetString("10000000000000000000", 10)
	return balance, nil
}

func (m *mockStore) Header() *types.Header {
	return &types.Header{}
}

type mockSigner struct{}

func (s *mockSigner) Sender(tx *types.Transaction) (types.Address, error) {
	return tx.From, nil
}

const validGasLimit uint64 = 100000

func TestAddingTransaction(t *testing.T) {
	senderPriv, _ := tests.GenerateKeyAndAddr(t)
	_, receiverAddr := tests.GenerateKeyAndAddr(t)

	testCases := []struct {
		name          string
		txValue       *big.Int
		txGasLimit    uint64
		txGasPrice    *big.Int
		shouldSucceed bool
	}{
		{
			name:          "transfer transaction",
			txValue:       big.NewInt(10),
			txGasLimit:    100000,
			txGasPrice:    big.NewInt(0),
			shouldSucceed: true,
		},
		{
			name:          "should fail with gas too low error",
			txValue:       big.NewInt(10),
			txGasLimit:    1,
			txGasPrice:    big.NewInt(1),
			shouldSucceed: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pool, err := NewTxPool(hclog.NewNullLogger(), false, nil, false, defaultPriceLimit, defaultMaxSlots, defaultSpeedUpMin, forks.At(0), &mockStore{}, nil, nil)
			if err != nil {
				t.Fatal("Failed to initialize transaction pool:", err)
			}
			signer := crypto.NewEIP155Signer(100)
			pool.AddSigner(signer)

			tx := &types.Transaction{
				To:       &receiverAddr,
				Value:    tc.txValue,
				Gas:      tc.txGasLimit,
				GasPrice: tc.txGasPrice,
			}

			signedTx, err := signer.SignTx(tx, senderPriv)
			if err != nil {
				t.Fatal("Failed to sign transaction:", err)
			}
			err = pool.AddTx(signedTx)

			if tc.shouldSucceed {
				assert.NoError(t, err, "Expected adding transaction to succeed")
				assert.NotEmpty(t, pool.Length(), "Expected pool to not be empty")
				assert.True(t, pool.pendingQueue.Contains(signedTx), "Expected pool to contain added transaction")
			} else {
				assert.ErrorIs(t, err, ErrIntrinsicGas, "Expected adding transaction to fail")
				assert.Empty(t, pool.Length(), "Expected pool to be empty")
			}
		})
	}
}

func TestMultipleTransactions(t *testing.T) {
	// if we add the same transaction it should only be included once
	pool, err := NewTxPool(hclog.NewNullLogger(), false, nil, true, defaultPriceLimit, defaultMaxSlots, defaultSpeedUpMin, forks.At(0), &mockStore{}, nil, nil)
	assert.NoError(t, err)
	pool.EnableDev()
	pool.AddSigner(&mockSigner{})

	from1 := types.Address{0x1}

	txn0 := &types.Transaction{
		From:     from1,
		Nonce:    10,
		Gas:      validGasLimit,
		GasPrice: big.NewInt(1),
		Value:    big.NewInt(0),
	}
	assert.NoError(t, pool.addImpl("", txn0))
	assert.NoError(t, pool.addImpl("", txn0))

	assert.Equal(t, pool.NumAccountTxs(from1), 1)
	assert.Equal(t, pool.Length(), uint64(0))

	from2 := types.Address{0x2}
	txn1 := &types.Transaction{
		From:     from2,
		Gas:      validGasLimit,
		GasPrice: big.NewInt(1),
		Value:    big.NewInt(0),
	}
	assert.NoError(t, pool.addImpl("", txn1))
	assert.NoError(t, pool.addImpl("", txn1))

	assert.Equal(t, pool.NumAccountTxs(from2), 0)
	assert.Equal(t, pool.Length(), uint64(1))
}

func TestGetPendingAndQueuedTransactions(t *testing.T) {
	pool, err := NewTxPool(hclog.NewNullLogger(), false, nil, false, defaultPriceLimit, defaultMaxSlots, defaultSpeedUpMin, forks.At(0), &mockStore{}, nil, nil)
	assert.NoError(t, err)
	pool.EnableDev()
	pool.AddSigner(&mockSigner{})

	from1 := types.Address{0x1}
	txn0 := &types.Transaction{
		From:     from1,
		Nonce:    0,
		Gas:      validGasLimit,
		Value:    big.NewInt(106),
		GasPrice: big.NewInt(1),
	}
	assert.NoError(t, pool.addImpl("", txn0))

	from2 := types.Address{0x2}
	txn1 := &types.Transaction{
		From:     from2,
		Nonce:    1,
		Gas:      validGasLimit,
		Value:    big.NewInt(106),
		GasPrice: big.NewInt(1),
	}
	assert.NoError(t, pool.addImpl("", txn1))

	from3 := types.Address{0x3}
	txn2 := &types.Transaction{
		From:     from3,
		Nonce:    2,
		Gas:      validGasLimit,
		Value:    big.NewInt(107),
		GasPrice: big.NewInt(1),
	}
	assert.NoError(t, pool.addImpl("", txn2))

	from4 := types.Address{0x4}
	txn3 := &types.Transaction{
		From:     from4,
		Nonce:    5,
		Gas:      validGasLimit,
		Value:    big.NewInt(108),
		GasPrice: big.NewInt(1),
	}
	assert.NoError(t, pool.addImpl("", txn3))

	pendingTxs, queuedTxs := pool.GetTxs()

	assert.Len(t, pendingTxs, 1)
	assert.Len(t, queuedTxs, 3)
	assert.Equal(t, pendingTxs[from1][txn0.Nonce].Value, big.NewInt(106))
}

func TestBroadcast(t *testing.T) {
	// we need a fully encrypted txn with (r, s, v) values so that we can
	// safely encrypt in RLP and broadcast it
	key0, addr0 := tests.GenerateKeyAndAddr(t)

	fmt.Println("-- addr")
	fmt.Println(addr0)

	signer := &crypto.FrontierSigner{}

	createPool := func() (*TxPool, *network.Server) {
		server := network.CreateServer(t, nil)
		pool, err := NewTxPool(hclog.NewNullLogger(), false, nil, true, defaultPriceLimit, defaultMaxSlots, defaultSpeedUpMin, forks.At(0), &mockStore{}, nil, server)
		assert.NoError(t, err)
		pool.AddSigner(signer)
		return pool, server
	}

	pool1, network1 := createPool()
	_, network2 := createPool()

	network.MultiJoin(t, network1, network2)

	// broadcast txn1 from pool1
	txn1 := &types.Transaction{
		Value:    big.NewInt(10),
		Gas:      validGasLimit,
		GasPrice: big.NewInt(1),
	}

	txn1, err := signer.SignTx(txn1, key0)
	assert.NoError(t, err)

	assert.NoError(t, pool1.AddTx(txn1))
	fmt.Println(pool1.Length())
}

func TestTxnQueue_Promotion(t *testing.T) {
	pool, err := NewTxPool(hclog.NewNullLogger(), false, nil, true, defaultPriceLimit, defaultMaxSlots, defaultSpeedUpMin, forks.At(0), &mockStore{}, nil, nil)
	assert.NoError(t, err)
	pool.EnableDev()
	pool.AddSigner(&mockSigner{})

	pool.addImpl("", &types.Transaction{
		From:     addr1,
		Gas:      validGasLimit,
		GasPrice: big.NewInt(1),
		Value:    big.NewInt(0),
	})

	nonce, _ := pool.GetNonce(addr1)
	assert.Equal(t, nonce, uint64(1))

	// though txn0 is not being processed yet and the current nonce is 0
	// we need to consider that txn0 is on the pendingQueue pool so this one is promoted too
	pool.addImpl("", &types.Transaction{
		From:     addr1,
		Nonce:    1,
		Gas:      validGasLimit,
		GasPrice: big.NewInt(1),
		Value:    big.NewInt(0),
	})

	nonce, _ = pool.GetNonce(addr1)
	assert.Equal(t, nonce, uint64(2))
	assert.Equal(t, pool.Length(), uint64(2))
}

func TestTxnQueue_Heap(t *testing.T) {
	type TestCase struct {
		From     types.Address
		Gas      uint64
		GasPrice *big.Int
		Nonce    uint64
		Index    int
		Value    *big.Int
	}

	test := func(t *testing.T, testTable []TestCase) {
		pool, err := NewTxPool(hclog.NewNullLogger(), false, nil, false, defaultPriceLimit, defaultMaxSlots, defaultSpeedUpMin, forks.At(0), &mockStore{}, nil, nil)
		assert.NoError(t, err)
		pool.EnableDev()
		pool.AddSigner(&mockSigner{})

		for _, testCase := range testTable {
			err := pool.addImpl("", &types.Transaction{
				From:     testCase.From,
				Gas:      testCase.Gas,
				GasPrice: testCase.GasPrice,
				Nonce:    testCase.Nonce,
				Value:    testCase.Value,
			})
			assert.NoError(t, err)
		}

		for _, testCase := range testTable {
			transaction, _ := pool.Pop()

			assert.NotNil(t, transaction)

			actual := TestCase{
				From:     transaction.From,
				Gas:      transaction.Gas,
				GasPrice: transaction.GasPrice,
				Nonce:    transaction.Nonce,
				Value:    transaction.Value,
			}

			assert.EqualValues(t, testCase, actual)
		}

		empty, _ := pool.Pop()
		assert.Nil(t, empty)
	}

	t.Run("the higher priced transaction should be popped first", func(t *testing.T) {
		test(t, []TestCase{
			{
				From:     addr1,
				Gas:      validGasLimit,
				GasPrice: big.NewInt(2),
				Value:    big.NewInt(0),
			},
			{
				From:     addr2,
				Gas:      validGasLimit,
				GasPrice: big.NewInt(1),
				Value:    big.NewInt(0),
			},
		})

	})

	t.Run("sort by nonce when same from address", func(t *testing.T) {
		test(t, []TestCase{
			{
				From:     addr1,
				Gas:      validGasLimit,
				GasPrice: big.NewInt(2),
				Nonce:    0,
				Value:    big.NewInt(0),
			},
			{
				From:     addr1,
				Gas:      validGasLimit,
				GasPrice: big.NewInt(3),
				Nonce:    1,
				Value:    big.NewInt(0),
			},
		})
	})

	t.Run("make sure that heap is not functioning as a FIFO", func(t *testing.T) {
		pool, err := NewTxPool(hclog.NewNullLogger(), false, nil, true, defaultPriceLimit, defaultMaxSlots, defaultSpeedUpMin, forks.At(0), &mockStore{}, nil, nil)
		assert.NoError(t, err)
		pool.EnableDev()
		pool.AddSigner(&mockSigner{})

		numTxns := 5
		txns := make([]*types.Transaction, numTxns)

		for i := 0; i < numTxns; i++ {
			txns[i] = &types.Transaction{
				From:     types.StringToAddress(strconv.Itoa(i + 1)),
				Gas:      validGasLimit,
				GasPrice: big.NewInt(int64(i + 1)),
				Value:    big.NewInt(0),
			}

			addErr := pool.addImpl("", txns[i])
			assert.Nilf(t, addErr, "Unable to add transaction to pool")
		}

		for i := numTxns - 1; i >= 0; i-- {
			txn, _ := pool.Pop()
			assert.Equalf(t, txns[i].GasPrice, txn.GasPrice, "Expected output mismatch")
		}
	})
}

func generateTx(from types.Address, value, gasPrice *big.Int, input []byte) *types.Transaction {
	return &types.Transaction{
		From:     from,
		Nonce:    0,
		Gas:      validGasLimit,
		GasPrice: gasPrice,
		Value:    value,
		Input:    input,
	}
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

func TestTxPool_ErrorCodes(t *testing.T) {
	testTable := []struct {
		name          string
		refAddress    types.Address
		txValue       *big.Int
		gasPrice      *big.Int
		mockStore     store
		expectedError error
		devMode       bool
	}{
		{
			// Transactions with a negative value should be discarded
			"ErrNegativeValue",
			types.Address{0x1},
			big.NewInt(-5),
			big.NewInt(1),
			&mockStore{},
			ErrNegativeValue,
			true,
		},
		{
			// Unencrypted transactions should be discarded if not in dev mode
			"ErrNonEncryptedTxn",
			types.Address{0x1},
			big.NewInt(0),
			big.NewInt(1),
			&mockStore{},
			ErrNonEncryptedTxn,
			false,
		},
		{
			// Transaction should have a valid sender encrypted if it is from a zeroAddress
			"ErrInvalidSender",
			types.ZeroAddress,
			big.NewInt(0),
			big.NewInt(1),
			&mockStore{},
			ErrInvalidSender,
			true,
		},
		{
			// Transaction should query valid account state
			"ErrInvalidAccountState",
			types.Address{0x1},
			big.NewInt(1),
			big.NewInt(1),
			&faultyMockStore{},
			ErrInvalidAccountState,
			true,
		},
		{
			// Transaction's GasPrice should exceed GasLimit in TxPool configuration
			"ErrUnderpriced",
			types.Address{0x1},
			big.NewInt(1),
			big.NewInt(0),
			&faultyMockStore{},
			ErrUnderpriced,
			true,
		},
	}

	for _, testCase := range testTable {
		t.Run(testCase.name, func(t *testing.T) {
			pool, err := NewTxPool(hclog.NewNullLogger(), false, nil, true, defaultPriceLimit, defaultMaxSlots, defaultSpeedUpMin, forks.At(0), testCase.mockStore, nil, nil)
			assert.NoError(t, err)
			if testCase.devMode {
				pool.EnableDev()
			}
			poolSigner := crypto.NewEIP155Signer(uint64(100))
			pool.AddSigner(poolSigner)

			refAddress := testCase.refAddress
			txn := generateTx(refAddress, testCase.txValue, testCase.gasPrice, nil)

			assert.ErrorIs(t, pool.addImpl("", txn), testCase.expectedError)

			assert.Nil(t, pool.accountQueues[refAddress])
			assert.Equal(t, pool.Length(), uint64(0))
		})
	}
}
func TestTx_MaxSize(t *testing.T) {
	pool, err := NewTxPool(hclog.NewNullLogger(), false, nil, false, defaultPriceLimit, defaultMaxSlots, defaultSpeedUpMin, forks.At(0), &mockStore{}, nil, nil)
	pool.EnableDev()
	pool.AddSigner(&mockSigner{})
	assert.NoError(t, err)
	pool.EnableDev()
	pool.AddSigner(&mockSigner{})

	tests := []struct {
		name    string
		address types.Address
		succeed bool
		size    uint64
	}{

		{
			name:    "Tx_Data is greater than MAX_SIZE",
			address: types.Address{0x1},
			succeed: false,
			size:    132096,
		},
		{
			name:    "Tx_Data is less than MAX_SIZE",
			address: types.Address{0x1},
			succeed: true,
			size:    1000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := make([]byte, tt.size)
			rand.Read(data)
			txn := generateTx(tt.address, big.NewInt(0), big.NewInt(1), data)
			err := pool.addImpl("", txn)
			if tt.succeed {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Equal(t, err, ErrOversizedData)
			}
		})
	}

}
func TestTxnOperatorAddNilRaw(t *testing.T) {
	pool, err := NewTxPool(hclog.NewNullLogger(), false, nil, true, defaultPriceLimit, defaultMaxSlots, defaultSpeedUpMin, forks.At(0), &mockStore{}, nil, nil)
	assert.NoError(t, err)

	txnReq := new(proto.AddTxnReq)
	response, err := pool.AddTxn(context.Background(), txnReq)
	assert.Errorf(t, err, "transaction's field raw is empty")
	assert.Nil(t, response)
}

func TestPriceLimit(t *testing.T) {
	signer := crypto.NewEIP155Signer(uint64(100))
	key, from := tests.GenerateKeyAndAddr(t)

	tests := []struct {
		name string
		// TxPool config
		noLocals   bool            // enables accepting all local transactions
		locals     []types.Address // white list
		priceLimit uint64
		// Tx
		origin   TxOrigin
		gasPrice *big.Int
		// Result
		err error
		len uint64
	}{
		// Local transactions
		{
			name:       "should accept local transaction",
			noLocals:   false,
			locals:     nil,
			priceLimit: 100000,
			origin:     OriginAddTxn,
			gasPrice:   big.NewInt(0),
			err:        nil,
			len:        1,
		},
		{
			name:       "should reject local transaction with lower gas price",
			noLocals:   true,
			locals:     nil,
			priceLimit: 100000,
			origin:     OriginAddTxn,
			gasPrice:   big.NewInt(0),
			err:        ErrUnderpriced,
			len:        0,
		},
		{
			name:       "should accept local transaction when NoLocals is enabled but account is in local addrs list",
			noLocals:   true,
			locals:     []types.Address{from},
			priceLimit: 100000,
			origin:     OriginAddTxn,
			gasPrice:   big.NewInt(0),
			err:        nil,
			len:        1,
		},
		// Remote transactions (Gossip)
		{
			name:       "should reject remote transaction (via Gossip) with lower gas price as default",
			noLocals:   false,
			locals:     nil,
			priceLimit: 100000,
			origin:     OriginGossip,
			gasPrice:   big.NewInt(0),
			err:        ErrUnderpriced,
			len:        0,
		},
		{
			name:       "should accept remote transaction (via Gossip) when account is in local addrs list",
			noLocals:   true,
			locals:     []types.Address{from},
			priceLimit: 100000,
			origin:     OriginGossip,
			gasPrice:   big.NewInt(0),
			err:        nil,
			len:        1,
		},
		// Remote Transaction (Reorg)
		{
			name:       "should reject remote transaction (by Reorg) with lower gas price as default",
			noLocals:   false,
			locals:     nil,
			priceLimit: 100000,
			origin:     OriginReorg,
			gasPrice:   big.NewInt(0),
			err:        ErrUnderpriced,
			len:        0,
		},
		{
			name:       "should accept remote transaction (by Reorg) when account is in local addrs list",
			noLocals:   true,
			locals:     []types.Address{from},
			priceLimit: 100000,
			origin:     OriginReorg,
			gasPrice:   big.NewInt(0),
			err:        nil,
			len:        1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool, err := NewTxPool(hclog.NewNullLogger(), false, tt.locals, tt.noLocals, tt.priceLimit, defaultMaxSlots, defaultSpeedUpMin, forks.At(0), &mockStore{}, nil, nil)
			assert.NoError(t, err)
			pool.AddSigner(signer)

			tx, err := signer.SignTx(&types.Transaction{
				To:       &addr1,
				Nonce:    0,
				Gas:      validGasLimit,
				GasPrice: tt.gasPrice,
				Value:    big.NewInt(0),
			}, key)
			assert.NoError(t, err)

			assert.Equal(t, tt.err, pool.addImpl(tt.origin, tx))
			assert.Equal(t, tt.len, pool.Length())
		})
	}
}

func TestSizeLimit(t *testing.T) {
	type account struct {
		key  *ecdsa.PrivateKey
		addr types.Address
	}
	type addTx struct {
		origin   TxOrigin
		account  *account
		nonce    uint64
		gasPrice *big.Int
		slot     uint64
	}

	signer := crypto.NewEIP155Signer(uint64(100))
	numAccounts := 3
	accounts := make([]*account, numAccounts)
	for i := range accounts {
		key, addr := tests.GenerateKeyAndAddr(t)
		accounts[i] = &account{
			key:  key,
			addr: addr,
		}
	}

	tests := []struct {
		name string
		// TxPool config
		maxSlot uint64
		//
		initialTxs []addTx
		// input
		input addTx
		// result
		err   error
		len   uint64
		slots uint64
	}{
		{
			name:       "should add new tx when tx pool has enough space",
			maxSlot:    5,
			initialTxs: nil,
			input: addTx{
				origin:   OriginAddTxn,
				account:  accounts[0],
				nonce:    0,
				gasPrice: big.NewInt(1),
				slot:     2,
			},
			err:   nil,
			len:   1,
			slots: 2,
		},
		{
			name:    "should reject new remote tx if txpool is full and the gas price is lower than any remote tx in the pool",
			maxSlot: 4,
			initialTxs: []addTx{
				{
					origin:   OriginGossip,
					account:  accounts[0],
					nonce:    0,
					gasPrice: big.NewInt(5),
					slot:     3,
				},
			},
			input: addTx{
				origin:   OriginGossip,
				account:  accounts[1],
				nonce:    0,
				gasPrice: big.NewInt(1),
				slot:     2,
			},
			err:   ErrUnderpriced,
			len:   1,
			slots: 3,
		},
		{
			name:    "should reject new remote tx if txpool is full and failed to make space",
			maxSlot: 4,
			initialTxs: []addTx{
				{
					origin:   OriginAddTxn,
					account:  accounts[0],
					nonce:    0,
					gasPrice: big.NewInt(5),
					slot:     3,
				},
			},
			input: addTx{
				origin:   OriginGossip,
				account:  accounts[1],
				nonce:    0,
				gasPrice: big.NewInt(1),
				slot:     2,
			},
			err:   ErrTxPoolOverflow,
			len:   1,
			slots: 3,
		},
		{
			name:    "should discard existing transactions if new tx set more expensive gas price",
			maxSlot: 4,
			initialTxs: []addTx{
				{
					origin:   OriginGossip,
					account:  accounts[0],
					nonce:    0,
					gasPrice: big.NewInt(1),
					slot:     3,
				},
			},
			input: addTx{
				origin:   OriginGossip,
				account:  accounts[1],
				nonce:    0,
				gasPrice: big.NewInt(5),
				slot:     2,
			},
			err:   nil,
			len:   1,
			slots: 2,
		},
		{
			name:    "should discard existing remote transactions and add new tx forcibly if the new tx is local and set more expensive gas price",
			maxSlot: 2,
			initialTxs: []addTx{
				{
					origin:   OriginGossip,
					account:  accounts[0],
					nonce:    0,
					gasPrice: big.NewInt(1),
					slot:     2,
				},
			},
			input: addTx{
				origin:   OriginAddTxn,
				account:  accounts[1],
				nonce:    0,
				gasPrice: big.NewInt(5),
				slot:     3,
			},
			err:   nil,
			len:   1,
			slots: 3,
		},
	}

	genTx := func(t *testing.T, arg *addTx) *types.Transaction {
		t.Helper()

		// base field should take 1 slot at least
		size := txSlotSize * (arg.slot - 1)
		if size <= 0 {
			size = 1
		}
		input := make([]byte, size)
		rand.Read(input)

		tx, err := signer.SignTx(&types.Transaction{
			To:       &addr1,
			Nonce:    arg.nonce,
			Gas:      100000000,
			GasPrice: arg.gasPrice,
			Value:    big.NewInt(0),
			Input:    input,
		}, arg.account.key)
		assert.NoError(t, err)
		return tx
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool, err := NewTxPool(hclog.NewNullLogger(), false, nil, false, defaultPriceLimit, tt.maxSlot, defaultSpeedUpMin, forks.At(0), &mockStore{}, nil, nil)
			assert.NoError(t, err)
			pool.AddSigner(signer)

			for _, arg := range tt.initialTxs {
				tx := genTx(t, &arg)
				assert.NoError(t, pool.addImpl(arg.origin, tx))
			}

			err = pool.addImpl(tt.input.origin, genTx(t, &tt.input))
			assert.Equal(t, tt.err, err)
			assert.Equal(t, tt.len, pool.Length())
			assert.Equal(t, tt.slots, pool.slots)
		})
	}
}

func TestSpeedUpTx(t *testing.T) {

	testTable := []struct {
		name          string
		speedUpAmount uint64
		nonce         uint64
		resendCount   int
		origin        TxOrigin
	}{
		{
			name:          "Speed up remote tx 5x",
			speedUpAmount: 500,
			nonce:         0,
			resendCount:   5,
			origin:        OriginGossip,
		},
		{
			name:          "Speed up remote tx 10x",
			speedUpAmount: 1000,
			nonce:         3,
			resendCount:   10,
			origin:        OriginGossip,
		},
		{
			name:          "Speed up local tx 5x",
			speedUpAmount: 500,
			nonce:         0,
			resendCount:   5,
			origin:        OriginAddTxn,
		},
		{
			name:          "Speed up local tx 10x",
			speedUpAmount: 1000,
			nonce:         3,
			resendCount:   10,
			origin:        OriginAddTxn,
		},
	}

	from := types.Address{0x1}
	for _, tt := range testTable {
		t.Run(tt.name, func(t *testing.T) {
			pool, err := NewTxPool(hclog.NewNullLogger(), false, nil, false, defaultPriceLimit, defaultMaxSlots, tt.speedUpAmount, forks.At(0), &mockStore{}, nil, nil)
			assert.NoError(t, err)
			pool.EnableDev()
			pool.AddSigner(&mockSigner{})

			// Add multiple same-nonce txs
			// but with higher gas price each time
			var highestBidder *big.Int
			for i := 0; i < tt.resendCount; i++ {
				tx := &types.Transaction{
					From:     from,
					Nonce:    tt.nonce,
					Gas:      validGasLimit,
					GasPrice: big.NewInt(0).SetUint64(10 + tt.speedUpAmount*uint64(i)),
					Value:    big.NewInt(0),
				}

				assert.NoError(t, pool.addImpl(tt.origin, tx))
				highestBidder = tx.GasPrice
			}

			var finalizedTx *types.Transaction
			if tt.nonce == 0 {
				// tx will get promoted as soon as its added,
				// so we check the pending queue
				assert.Equal(t, pool.NumAccountTxs(from), 0)
				assert.Equal(t, pool.Length(), uint64(1))

				finalizedTx, _ = pool.Pop()
			} else {
				// otherwise it will reside in the account queue
				// waiting on promotion
				assert.Equal(t, pool.NumAccountTxs(from), 1)
				assert.Equal(t, pool.Length(), uint64(0))

				finalizedTx = pool.accountQueues[from].accountQueue.Pop()
			}

			assert.NotNil(t, finalizedTx)
			assert.Equal(t, finalizedTx.GasPrice.Uint64(), highestBidder.Uint64())
		})
	}
}

func TestRejectUnderpricedSpeedUp(t *testing.T) {
	testTable := []struct {
		name          string
		speedUpAmount uint64
		nonce         uint64
		origin        TxOrigin
	}{
		{
			name:          "Reject underpriced remote tx from account queue",
			speedUpAmount: 50,
			nonce:         10,
			origin:        OriginGossip,
		},
		{
			name:          "Reject underpriced remote tx from promoted queue",
			speedUpAmount: 100,
			nonce:         0,
			origin:        OriginGossip,
		},
		{
			name:          "Reject underpriced local tx from account queue",
			speedUpAmount: 50,
			nonce:         10,
			origin:        OriginAddTxn,
		},
		{
			name:          "Reject underpriced local tx from promoted queue",
			speedUpAmount: 100,
			nonce:         0,
			origin:        OriginAddTxn,
		},
	}

	from := types.Address{0x1}
	for _, tt := range testTable {
		t.Run(tt.name, func(t *testing.T) {
			pool, err := NewTxPool(hclog.NewNullLogger(), false, nil, false, defaultPriceLimit, defaultMaxSlots, tt.speedUpAmount, forks.At(0), &mockStore{}, nil, nil)
			assert.NoError(t, err)
			pool.EnableDev()
			pool.AddSigner(&mockSigner{})

			tx0 := &types.Transaction{
				From:     from,
				Nonce:    tt.nonce,
				Gas:      validGasLimit,
				GasPrice: big.NewInt(1),
				Value:    big.NewInt(0),
			}
			assert.NoError(t, pool.addImpl(tt.origin, tx0))

			tx1 := &types.Transaction{
				From:     from,
				Nonce:    tt.nonce,
				Gas:      validGasLimit,
				GasPrice: big.NewInt(1), // same gas price as previous
				Value:    big.NewInt(0),
			}
			assert.ErrorIs(t, ErrUnderpriced, pool.addImpl(tt.origin, tx1))

			tx2 := &types.Transaction{
				From:     from,
				Nonce:    tt.nonce,
				Gas:      validGasLimit,
				GasPrice: big.NewInt(0).SetUint64(1 + tt.speedUpAmount/2), // higher, but still insufficient
				Value:    big.NewInt(0),
			}
			assert.ErrorIs(t, ErrUnderpriced, pool.addImpl(tt.origin, tx2))

			if tt.nonce == 0 {
				// tx will get promoted as soon as its added,
				// so we check the pending queue
				assert.Equal(t, pool.NumAccountTxs(from), 0)
				assert.Equal(t, pool.Length(), uint64(1))
			} else {
				// otherwise it will reside in the account queue
				// waiting on promotion
				assert.Equal(t, pool.NumAccountTxs(from), 1)
				assert.Equal(t, pool.Length(), uint64(0))
			}
		})
	}
}
