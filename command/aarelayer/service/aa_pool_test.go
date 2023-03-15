package service

import (
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

	require.Nil(t, aaPool.Pop())
}

func checkPops(t *testing.T, aaPool AAPool) {
	t.Helper()

	require.Equal(t, 5, aaPool.Len())

	item := aaPool.Pop()
	require.Equal(t, types.StringToAddress("cc"), item.Tx.Transaction.From)
	require.Equal(t, uint64(1), item.Tx.Transaction.Nonce)
	require.Equal(t, 4, aaPool.Len())

	aaPool.Push(&AAStateTransaction{
		Tx: &AATransaction{
			Transaction: Transaction{Nonce: 3, From: types.StringToAddress("cc")},
		},
		Time: 180,
	})

	item = aaPool.Pop()
	require.Equal(t, types.StringToAddress("aa"), item.Tx.Transaction.From)
	require.Equal(t, 4, aaPool.Len())

	item = aaPool.Pop()
	require.Equal(t, types.StringToAddress("cc"), item.Tx.Transaction.From)
	require.Equal(t, uint64(2), item.Tx.Transaction.Nonce)
	require.Equal(t, 3, aaPool.Len())

	for i := 1; i <= 2; i++ {
		item = aaPool.Pop()
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

	item = aaPool.Pop()
	require.Equal(t, types.StringToAddress("cc"), item.Tx.Transaction.From)
	require.Equal(t, uint64(3), item.Tx.Transaction.Nonce)
	require.Equal(t, 0, aaPool.Len())
}

func getDummyTxs() []*AAStateTransaction {
	return []*AAStateTransaction{
		{
			Tx: &AATransaction{
				Transaction: Transaction{Nonce: 1, From: types.StringToAddress("ff")},
			},
			Time: 60,
		},
		{
			Tx: &AATransaction{
				Transaction: Transaction{Nonce: 2, From: types.StringToAddress("cc")},
			},
			Time: 45,
		},
		{
			Tx: &AATransaction{
				Transaction: Transaction{Nonce: 1, From: types.StringToAddress("aa")},
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
