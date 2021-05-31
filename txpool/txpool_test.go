package txpool

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/0xPolygon/minimal/crypto"
	"github.com/0xPolygon/minimal/network"
	"github.com/0xPolygon/minimal/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
)

func TestMultipleTransactions(t *testing.T) {
	// if we add the same transaction it should only be included once
	pool, err := NewTxPool(hclog.NewNullLogger(), false, &mockStore{}, nil, nil)
	assert.NoError(t, err)
	pool.EnableDev()

	from1 := types.Address{0x1}

	txn0 := &types.Transaction{
		From:     from1,
		Nonce:    10,
		GasPrice: big.NewInt(1),
	}
	assert.NoError(t, pool.addImpl("", txn0))
	assert.NoError(t, pool.addImpl("", txn0))

	assert.Len(t, pool.queue[from1].txs, 1)
	assert.Equal(t, pool.Length(), uint64(0))

	from2 := types.Address{0x2}
	txn1 := &types.Transaction{
		From:     from2,
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
	key0, _ := crypto.GenerateKey()
	addr0 := crypto.PubKeyToAddress(&key0.PublicKey)

	fmt.Println("-- addr")
	fmt.Println(addr0)

	signer := &crypto.FrontierSigner{}

	createPool := func() *TxPool {
		pool, err := NewTxPool(hclog.NewNullLogger(), false, &mockStore{}, nil, network.CreateServer(t, nil))
		assert.NoError(t, err)
		pool.AddSigner(signer)
		return pool
	}

	pool1 := createPool()
	pool2 := createPool()

	network.MultiJoin(t, pool1.network, pool2.network)

	// broadcast txn1 from pool1
	txn1 := &types.Transaction{
		Value:    big.NewInt(10),
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
	pool, err := NewTxPool(hclog.NewNullLogger(), false, &mockStore{}, nil, nil)
	assert.NoError(t, err)
	pool.EnableDev()

	addr1 := types.Address{0x1}

	pool.addImpl("", &types.Transaction{
		From:     addr1,
		GasPrice: big.NewInt(1),
	})

	nonce, _ := pool.GetNonce(addr1)
	assert.Equal(t, nonce, uint64(1))

	// though txn0 is not being processed yet and the current nonce is 0
	// we need to consider that txn0 is on the sorted pool so this one is promoted too
	pool.addImpl("", &types.Transaction{
		From:     addr1,
		Nonce:    1,
		GasPrice: big.NewInt(1),
	})

	nonce, _ = pool.GetNonce(addr1)
	assert.Equal(t, nonce, uint64(2))
	assert.Equal(t, pool.Length(), uint64(2))
}
