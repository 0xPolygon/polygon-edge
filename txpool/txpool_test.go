package txpool

import (
	"fmt"
	"github.com/0xPolygon/minimal/chain"
	"math/big"
	"strconv"
	"testing"

	"github.com/0xPolygon/minimal/crypto"
	"github.com/0xPolygon/minimal/helper/tests"
	"github.com/0xPolygon/minimal/network"
	"github.com/0xPolygon/minimal/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
)

var forks = &chain.Forks{
	Homestead: chain.NewFork(0),
	Istanbul:  chain.NewFork(0),
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
			pool, err := NewTxPool(hclog.NewNullLogger(), false, forks.At(0), &mockStore{}, nil, nil)
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
				assert.True(t, pool.sorted.Contains(signedTx), "Expected pool to contain added transaction")
			} else {
				assert.ErrorIs(t, err, ErrIntrinsicGas, "Expected adding transaction to fail")
				assert.Empty(t, pool.Length(), "Expected pool to be empty")
			}
		})
	}
}

func TestMultipleTransactions(t *testing.T) {
	// if we add the same transaction it should only be included once
	pool, err := NewTxPool(hclog.NewNullLogger(), false, forks.At(0), &mockStore{}, nil, nil)
	assert.NoError(t, err)
	pool.EnableDev()

	from1 := types.Address{0x1}

	txn0 := &types.Transaction{
		From:     from1,
		Nonce:    10,
		Gas:      validGasLimit,
		GasPrice: big.NewInt(1),
	}
	assert.NoError(t, pool.addImpl("", txn0))
	assert.NoError(t, pool.addImpl("", txn0))

	assert.Len(t, pool.queue[from1].txs, 1)
	assert.Equal(t, pool.Length(), uint64(0))

	from2 := types.Address{0x2}
	txn1 := &types.Transaction{
		From:     from2,
		Gas:      validGasLimit,
		GasPrice: big.NewInt(1),
	}
	assert.NoError(t, pool.addImpl("", txn1))
	assert.NoError(t, pool.addImpl("", txn1))

	assert.Len(t, pool.queue[from2].txs, 0)
	assert.Equal(t, pool.Length(), uint64(1))
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
		pool, err := NewTxPool(hclog.NewNullLogger(), false, forks.At(0), &mockStore{}, nil, server)
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

type mockStore struct {
}

func (m *mockStore) GetNonce(root types.Hash, addr types.Address) uint64 {
	return 0
}

func (m *mockStore) GetBlockByHash(types.Hash, bool) (*types.Block, bool) {
	return nil, false
}

func (m *mockStore) Header() *types.Header {
	return &types.Header{}
}

func TestTxnQueue_Promotion(t *testing.T) {
	pool, err := NewTxPool(hclog.NewNullLogger(), false, forks.At(0), &mockStore{}, nil, nil)
	assert.NoError(t, err)
	pool.EnableDev()

	addr1 := types.Address{0x1}

	pool.addImpl("", &types.Transaction{
		From:     addr1,
		Gas:      validGasLimit,
		GasPrice: big.NewInt(1),
	})

	nonce, _ := pool.GetNonce(addr1)
	assert.Equal(t, nonce, uint64(1))

	// though txn0 is not being processed yet and the current nonce is 0
	// we need to consider that txn0 is on the sorted pool so this one is promoted too
	pool.addImpl("", &types.Transaction{
		From:     addr1,
		Nonce:    1,
		Gas:      validGasLimit,
		GasPrice: big.NewInt(1),
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
	}

	addr1 := types.Address{0x1}
	addr2 := types.Address{0x2}

	test := func(t *testing.T, testTable []TestCase) {
		pool, err := NewTxPool(hclog.NewNullLogger(), false, forks.At(0), &mockStore{}, nil, nil)
		assert.NoError(t, err)
		pool.EnableDev()

		for _, testCase := range testTable {
			err := pool.addImpl("", &types.Transaction{
				From:     testCase.From,
				Gas:      testCase.Gas,
				GasPrice: testCase.GasPrice,
				Nonce:    testCase.Nonce,
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
			},
			{
				From:     addr2,
				Gas:      validGasLimit,
				GasPrice: big.NewInt(1),
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
			},
			{
				From:     addr1,
				Gas:      validGasLimit,
				GasPrice: big.NewInt(3),
				Nonce:    1,
			},
		})
	})

	t.Run("make sure that heap is not functioning as a FIFO", func(t *testing.T) {
		pool, err := NewTxPool(hclog.NewNullLogger(), false, forks.At(0), &mockStore{}, nil, nil)
		assert.NoError(t, err)
		pool.EnableDev()

		numTxns := 5
		txns := make([]*types.Transaction, numTxns)

		for i := 0; i < numTxns; i++ {
			txns[i] = &types.Transaction{
				From:     types.StringToAddress(strconv.Itoa(i + 1)),
				Gas:      validGasLimit,
				GasPrice: big.NewInt(int64(i + 1)),
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
