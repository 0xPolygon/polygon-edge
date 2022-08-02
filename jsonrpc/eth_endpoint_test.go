package jsonrpc

import (
	"errors"
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
)

func TestEth_DecodeTxn(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		accounts map[types.Address]*state.Account
		arg      *txnArgs
		res      *types.Transaction
		err      error
	}{
		{
			name: "should be failed when both To and Data doesn't set",
			arg: &txnArgs{
				From:     &addr1,
				Gas:      toArgUint64Ptr(21000),
				GasPrice: toArgBytesPtr(big.NewInt(10000).Bytes()),
				Value:    toArgBytesPtr(oneEther.Bytes()),
				Nonce:    toArgUint64Ptr(0),
			},
			res: nil,
			err: errors.New("contract creation without data provided"),
		},
		{
			name: "should be successful",
			arg: &txnArgs{
				From:     &addr1,
				To:       &addr2,
				Gas:      toArgUint64Ptr(21000),
				GasPrice: toArgBytesPtr(big.NewInt(10000).Bytes()),
				Value:    toArgBytesPtr(oneEther.Bytes()),
				Data:     nil,
				Nonce:    toArgUint64Ptr(0),
			},
			res: &types.Transaction{
				From:     addr1,
				To:       &addr2,
				Gas:      21000,
				GasPrice: big.NewInt(10000),
				Value:    oneEther,
				Input:    []byte{},
				Nonce:    0,
			},
			err: nil,
		},
		{
			name: "should set zero address and zero nonce as default",
			arg: &txnArgs{
				To:       &addr2,
				Gas:      toArgUint64Ptr(21000),
				GasPrice: toArgBytesPtr(big.NewInt(10000).Bytes()),
				Value:    toArgBytesPtr(oneEther.Bytes()),
				Data:     nil,
			},
			res: &types.Transaction{
				From:     types.ZeroAddress,
				To:       &addr2,
				Gas:      21000,
				GasPrice: big.NewInt(10000),
				Value:    oneEther,
				Input:    []byte{},
				Nonce:    0,
			},
			err: nil,
		},
		{
			name: "should set latest nonce as default",
			accounts: map[types.Address]*state.Account{
				addr1: {
					Nonce: 10,
				},
			},
			arg: &txnArgs{
				From:     &addr1,
				To:       &addr2,
				Gas:      toArgUint64Ptr(21000),
				GasPrice: toArgBytesPtr(big.NewInt(10000).Bytes()),
				Value:    toArgBytesPtr(oneEther.Bytes()),
				Data:     nil,
			},
			res: &types.Transaction{
				From:     addr1,
				To:       &addr2,
				Gas:      21000,
				GasPrice: big.NewInt(10000),
				Value:    oneEther,
				Input:    []byte{},
				Nonce:    10,
			},
			err: nil,
		},
		{
			name: "should set empty bytes as default Input",
			arg: &txnArgs{
				From:     &addr1,
				To:       &addr2,
				Gas:      toArgUint64Ptr(21000),
				GasPrice: toArgBytesPtr(big.NewInt(10000).Bytes()),
				Data:     nil,
				Nonce:    toArgUint64Ptr(1),
			},
			res: &types.Transaction{
				From:     addr1,
				To:       &addr2,
				Gas:      21000,
				GasPrice: big.NewInt(10000),
				Value:    new(big.Int).SetBytes([]byte{}),
				Input:    []byte{},
				Nonce:    1,
			},
			err: nil,
		},
		{
			name: "should set zero as default Gas",
			arg: &txnArgs{
				From:     &addr1,
				To:       &addr2,
				GasPrice: toArgBytesPtr(big.NewInt(10000).Bytes()),
				Data:     nil,
				Nonce:    toArgUint64Ptr(1),
			},
			res: &types.Transaction{
				From:     addr1,
				To:       &addr2,
				Gas:      0,
				GasPrice: big.NewInt(10000),
				Value:    new(big.Int).SetBytes([]byte{}),
				Input:    []byte{},
				Nonce:    1,
			},
			err: nil,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if tt.res != nil {
				tt.res.ComputeHash()
			}
			store := newMockStore()
			for addr, acc := range tt.accounts {
				store.SetAccount(addr, acc)
			}

			eth := newTestEthEndpoint(store)
			res, err := eth.decodeTxn(tt.arg)
			assert.Equal(t, tt.res, res)
			assert.Equal(t, tt.err, err)
		})
	}
}

func TestEth_GetNextNonce(t *testing.T) {
	t.Parallel()

	// Set up the mock accounts
	accounts := []struct {
		address types.Address
		account *state.Account
	}{
		{
			types.StringToAddress("123"),
			&state.Account{
				Nonce: 5,
			},
		},
	}

	// Set up the mock store
	store := newMockStore()
	for _, acc := range accounts {
		store.SetAccount(acc.address, acc.account)
	}

	eth := newTestEthEndpoint(store)

	testTable := []struct {
		name          string
		account       types.Address
		number        BlockNumber
		expectedNonce uint64
	}{
		{
			"Valid latest nonce for touched account",
			accounts[0].address,
			LatestBlockNumber,
			accounts[0].account.Nonce,
		},
		{
			"Valid latest nonce for untouched account",
			types.StringToAddress("456"),
			LatestBlockNumber,
			0,
		},
		{
			"Valid pending nonce for untouched account",
			types.StringToAddress("789"),
			LatestBlockNumber,
			0,
		},
	}

	for _, testCase := range testTable {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			// Grab the nonce
			nonce, err := eth.getNextNonce(testCase.account, testCase.number)

			// Assert errors
			assert.NoError(t, err)

			// Assert equality
			assert.Equal(t, testCase.expectedNonce, nonce)
		})
	}
}

func newTestEthEndpoint(store ethStore) *Eth {
	return &Eth{hclog.NewNullLogger(), store, 100, nil, 0}
}

func newTestEthEndpointWithPriceLimit(store ethStore, priceLimit uint64) *Eth {
	return &Eth{hclog.NewNullLogger(), store, 100, nil, priceLimit}
}
