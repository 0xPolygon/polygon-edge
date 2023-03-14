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

	require.Nil(t, aaPool.Pop())
}

func checkPops(t *testing.T, aaPool AAPool) {
	t.Helper()

	require.Equal(t, 5, aaPool.Len())

	for i := 1; i <= 2; i++ {
		item := aaPool.Pop()
		require.Equal(t, types.StringToAddress("cc"), item.Tx.Transaction.From)
		require.Equal(t, uint64(i), item.Tx.Transaction.Nonce)
		require.Equal(t, 5-i, aaPool.Len())
	}

	item := aaPool.Pop()
	require.Equal(t, types.StringToAddress("aa"), item.Tx.Transaction.From)
	require.Equal(t, 2, aaPool.Len())

	for i := 1; i <= 2; i++ {
		item = aaPool.Pop()
		require.Equal(t, types.StringToAddress("ff"), item.Tx.Transaction.From)
		require.Equal(t, uint64(i), item.Tx.Transaction.Nonce)
		require.Equal(t, 2-i, aaPool.Len())
	}
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
			Time: 1,
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
