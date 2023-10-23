package jsonrpc

import (
	"errors"
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEth_DecodeTxn(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		accounts map[types.Address]*Account
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
				From:      &addr1,
				To:        &addr2,
				Gas:       toArgUint64Ptr(21000),
				GasPrice:  toArgBytesPtr(big.NewInt(10000).Bytes()),
				GasTipCap: toArgBytesPtr(big.NewInt(10000).Bytes()),
				GasFeeCap: toArgBytesPtr(big.NewInt(10000).Bytes()),
				Value:     toArgBytesPtr(oneEther.Bytes()),
				Data:      nil,
				Nonce:     toArgUint64Ptr(0),
			},
			res: &types.Transaction{
				From:      addr1,
				To:        &addr2,
				Gas:       21000,
				GasPrice:  big.NewInt(10000),
				GasTipCap: big.NewInt(10000),
				GasFeeCap: big.NewInt(10000),
				Value:     oneEther,
				Input:     []byte{},
				Nonce:     0,
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
				From:      types.ZeroAddress,
				To:        &addr2,
				Gas:       21000,
				GasPrice:  big.NewInt(10000),
				GasTipCap: new(big.Int),
				GasFeeCap: new(big.Int),
				Value:     oneEther,
				Input:     []byte{},
				Nonce:     0,
			},
			err: nil,
		},
		{
			name: "should set latest nonce as default",
			accounts: map[types.Address]*Account{
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
				From:      addr1,
				To:        &addr2,
				Gas:       21000,
				GasPrice:  big.NewInt(10000),
				GasTipCap: new(big.Int),
				GasFeeCap: new(big.Int),
				Value:     oneEther,
				Input:     []byte{},
				Nonce:     10,
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
				From:      addr1,
				To:        &addr2,
				Gas:       21000,
				GasPrice:  big.NewInt(10000),
				GasTipCap: new(big.Int),
				GasFeeCap: new(big.Int),
				Value:     new(big.Int).SetBytes([]byte{}),
				Input:     []byte{},
				Nonce:     1,
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
				From:      addr1,
				To:        &addr2,
				Gas:       0,
				GasPrice:  big.NewInt(10000),
				GasTipCap: new(big.Int),
				GasFeeCap: new(big.Int),
				Value:     new(big.Int).SetBytes([]byte{}),
				Input:     []byte{},
				Nonce:     1,
			},
			err: nil,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if tt.res != nil {
				tt.res.ComputeHash(1)
			}

			store := newMockStore()
			for addr, acc := range tt.accounts {
				store.SetAccount(addr, acc)
			}

			res, err := DecodeTxn(tt.arg, 1, store, false)
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
		account *Account
	}{
		{
			types.StringToAddress("123"),
			&Account{
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
			nonce, err := GetNextNonce(testCase.account, testCase.number, eth.store)

			// Assert errors
			assert.NoError(t, err)

			// Assert equality
			assert.Equal(t, testCase.expectedNonce, nonce)
		})
	}
}

func TestEth_TxnType(t *testing.T) {
	// Set up the mock accounts
	accounts := []struct {
		address types.Address
		account *Account
	}{
		{
			types.StringToAddress("123"),
			&Account{
				Nonce: 5,
			},
		},
	}

	// Set up the mock store
	store := newMockStore()
	for _, acc := range accounts {
		store.SetAccount(acc.address, acc.account)
	}

	// Setup Txn
	args := &txnArgs{
		From:      &addr1,
		To:        &addr2,
		Gas:       toArgUint64Ptr(21000),
		GasPrice:  toArgBytesPtr(big.NewInt(10000).Bytes()),
		GasTipCap: toArgBytesPtr(big.NewInt(10000).Bytes()),
		GasFeeCap: toArgBytesPtr(big.NewInt(10000).Bytes()),
		Value:     toArgBytesPtr(oneEther.Bytes()),
		Data:      nil,
		Nonce:     toArgUint64Ptr(0),
		Type:      toArgUint64Ptr(uint64(types.DynamicFeeTx)),
	}

	expectedRes := &types.Transaction{
		From:      addr1,
		To:        &addr2,
		Gas:       21000,
		GasPrice:  big.NewInt(10000),
		GasTipCap: big.NewInt(10000),
		GasFeeCap: big.NewInt(10000),
		Value:     oneEther,
		Input:     []byte{},
		Nonce:     0,
		Type:      types.DynamicFeeTx,
	}
	res, err := DecodeTxn(args, 1, store, false)

	expectedRes.ComputeHash(1)
	assert.NoError(t, err)
	assert.Equal(t, expectedRes, res)
}

func newTestEthEndpoint(store testStore) *Eth {
	return &Eth{
		hclog.NewNullLogger(), store, 100, nil, 0,
	}
}

func newTestEthEndpointWithPriceLimit(store testStore, priceLimit uint64) *Eth {
	return &Eth{
		hclog.NewNullLogger(), store, 100, nil, priceLimit,
	}
}

func TestEth_HeaderResolveBlock(t *testing.T) {
	// Set up the mock store
	store := newMockStore()
	store.header.Number = 10

	latest := LatestBlockNumber
	blockNum5 := BlockNumber(5)
	blockNum10 := BlockNumber(10)
	hash := types.Hash{0x1}

	cases := []struct {
		filter BlockNumberOrHash
		err    bool
	}{
		{
			// both fields are empty
			BlockNumberOrHash{},
			false,
		},
		{
			// return the latest block number
			BlockNumberOrHash{BlockNumber: &latest},
			false,
		},
		{
			// specific real block number
			BlockNumberOrHash{BlockNumber: &blockNum10},
			false,
		},
		{
			// specific block number (not found)
			BlockNumberOrHash{BlockNumber: &blockNum5},
			true,
		},
		{
			// specific block by hash (found). By default all blocks in the mock have hash zero
			BlockNumberOrHash{BlockHash: &types.ZeroHash},
			false,
		},
		{
			// specific block by hash (not found)
			BlockNumberOrHash{BlockHash: &hash},
			true,
		},
	}

	for _, c := range cases {
		header, err := GetHeaderFromBlockNumberOrHash(c.filter, store)
		if c.err {
			assert.Error(t, err)
		} else {
			assert.NoError(t, err)
			assert.Equal(t, header.Number, uint64(10))
		}
	}
}

func TestOverrideAccount_ToType(t *testing.T) {
	t.Parallel()

	nonce := uint64(10)
	code := []byte("SC code")
	balance := uint64(10000)
	state := map[types.Hash]types.Hash{types.StringToHash("1"): types.StringToHash("2")}
	stateDiff := map[types.Hash]types.Hash{types.StringToHash("3"): types.StringToHash("4")}

	overrideAcc := &overrideAccount{
		Nonce:     toArgUint64Ptr(nonce),
		Code:      toArgBytesPtr(code),
		Balance:   toArgUint64Ptr(balance),
		State:     &state,
		StateDiff: &stateDiff,
	}

	convertedAcc := overrideAcc.ToType()
	require.NotNil(t, convertedAcc)
	require.Equal(t, nonce, *convertedAcc.Nonce)
	require.Equal(t, code, convertedAcc.Code)
	require.Equal(t, new(big.Int).SetUint64(balance), convertedAcc.Balance)
	require.Equal(t, state, convertedAcc.State)
	require.Equal(t, stateDiff, convertedAcc.StateDiff)
}
