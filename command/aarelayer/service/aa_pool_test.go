package service

import (
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/require"
)

func Test_AAPool_Init(t *testing.T) {
	t.Parallel()

	aaPool := NewAAPool()
	aaPool.Init(getDummyTxs())

	checkPops(t, aaPool)
}

func Test_AAPool_Push_Pop(t *testing.T) {
	t.Parallel()

	aaPool := NewAAPool()
	require.Equal(t, 0, aaPool.Len())

	for _, x := range getDummyTxs() {
		aaPool.Push(x)
	}

	checkPops(t, aaPool)

	require.Nil(t, pop(aaPool))
}

func checkPops(t *testing.T, aaPool AAPool) {
	t.Helper()

	require.Equal(t, 5, aaPool.Len())

	item := pop(aaPool)
	require.Equal(t, types.StringToAddress("cc"), item.Tx.Transaction.From)
	require.Equal(t, uint64(1), item.Tx.Transaction.Nonce)
	require.Equal(t, 4, aaPool.Len())

	aaPool.Push(&AAStateTransaction{
		Tx: &AATransaction{
			Transaction: Transaction{Nonce: 3, From: types.StringToAddress("cc")},
		},
		Time: 180,
	})

	item = pop(aaPool)
	require.Equal(t, types.StringToAddress("aa"), item.Tx.Transaction.From)
	require.Equal(t, 4, aaPool.Len())

	item = pop(aaPool)
	require.Equal(t, types.StringToAddress("cc"), item.Tx.Transaction.From)
	require.Equal(t, uint64(2), item.Tx.Transaction.Nonce)
	require.Equal(t, 3, aaPool.Len())

	for i := 1; i <= 2; i++ {
		item = pop(aaPool)
		require.Equal(t, types.StringToAddress("ff"), item.Tx.Transaction.From)
		require.Equal(t, uint64(i), item.Tx.Transaction.Nonce)
		require.Equal(t, 3-i, aaPool.Len())
	}

	aaPool.Push(&AAStateTransaction{
		Tx: &AATransaction{
			Transaction: Transaction{Nonce: 3, From: types.StringToAddress("ff")},
		},
		Time: 120,
	})

	item = aaPool.Pop()
	require.Equal(t, types.StringToAddress("ff"), item.Tx.Transaction.From)
	require.Equal(t, uint64(3), item.Tx.Transaction.Nonce)
	require.Equal(t, 1, aaPool.Len())

	aaPool.Push(&AAStateTransaction{
		Tx: &AATransaction{
			Transaction: Transaction{Nonce: 4, From: types.StringToAddress("ff")},
		},
		Time: 190,
	})

	item = pop(aaPool)
	require.Equal(t, types.StringToAddress("cc"), item.Tx.Transaction.From)
	require.Equal(t, uint64(3), item.Tx.Transaction.Nonce)
	require.Equal(t, 1, aaPool.Len())

	item = pop(aaPool)
	require.Equal(t, types.StringToAddress("ff"), item.Tx.Transaction.From)
	require.Equal(t, uint64(4), item.Tx.Transaction.Nonce)
	require.Equal(t, 0, aaPool.Len())
}

func pop(aaPool AAPool) *AAStateTransaction {
	tx := aaPool.Pop()

	if tx != nil {
		aaPool.Update(tx.Tx.Transaction.From)
	}

	return tx
}

func getDummyTxs() []*AAStateTransaction {
	dummyAddress1 := types.StringToAddress("87830111")
	dummyAddress2 := types.StringToAddress("87830111")

	return []*AAStateTransaction{
		{
			Tx: &AATransaction{
				Transaction: Transaction{Nonce: 1, From: types.StringToAddress("ff")},
			},
			Time: 60,
		},
		{
			Tx: &AATransaction{
				Transaction: Transaction{
					Nonce: 2, From: types.StringToAddress("cc"),
					Payload: []Payload{
						{
							To:       &dummyAddress1,
							Value:    big.NewInt(10),
							GasLimit: big.NewInt(20),
						},
					},
				},
			},
			Time: 45,
		},
		{
			Tx: &AATransaction{
				Transaction: Transaction{
					Nonce: 1, From: types.StringToAddress("aa"),
					Payload: []Payload{
						{
							To:       &dummyAddress2,
							Value:    big.NewInt(1),
							GasLimit: big.NewInt(21000),
						},
						{
							To:       nil,
							Value:    big.NewInt(100),
							GasLimit: big.NewInt(201),
							Input:    []byte{1, 2, 3},
						},
					},
				},
			},
			Time: 40,
		},
		{
			Tx: &AATransaction{
				Transaction: Transaction{Nonce: 2, From: types.StringToAddress("ff")},
			},
			Time: 50,
		},
		{
			Tx: &AATransaction{
				Transaction: Transaction{Nonce: 1, From: types.StringToAddress("cc")},
			},
			Time: 10,
		},
	}
}
