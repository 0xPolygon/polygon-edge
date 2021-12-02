package jsonrpc

import (
	"encoding/json"
	"errors"
	"math/big"
	"reflect"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-sdk/state"
	"github.com/0xPolygon/polygon-sdk/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
)

var (
	oneEther = new(big.Int).Mul(
		big.NewInt(1),
		new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))
)

func toArgUint64Ptr(value uint64) *argUint64 {
	argValue := argUint64(value)
	return &argValue
}

func toArgBytesPtr(value []byte) *argBytes {
	argValue := argBytes(value)
	return &argValue
}

func expectJSONResult(data []byte, v interface{}) error {
	var resp SuccessResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return err
	}
	if resp.Error != nil {
		return resp.Error
	}
	if err := json.Unmarshal(resp.Result, v); err != nil {
		return err
	}
	return nil
}

func expectBatchJSONResult(data []byte, v interface{}) error {
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	return nil
}

func TestDispatcherWebsocket(t *testing.T) {
	store := newMockStore()

	s := newDispatcher(hclog.NewNullLogger(), store, 0)
	s.registerEndpoints()

	mock := &mockWsConn{
		msgCh: make(chan []byte, 1),
	}

	req := []byte(`{
		"method": "eth_subscribe",
		"params": ["newHeads"]
	}`)
	if _, err := s.HandleWs(req, mock); err != nil {
		t.Fatal(err)
	}

	store.emitEvent(&mockEvent{
		NewChain: []*mockHeader{
			{
				header: &types.Header{
					Hash: types.StringToHash("1"),
				},
			},
		},
	})

	select {
	case <-mock.msgCh:
	case <-time.After(2 * time.Second):
		t.Fatal("bad")
	}
}

func TestDispatcherWebsocketRequestFormats(t *testing.T) {
	store := newMockStore()

	s := newDispatcher(hclog.NewNullLogger(), store, 0)
	s.registerEndpoints()

	mock := &mockWsConn{
		msgCh: make(chan []byte, 1),
	}

	cases := []struct {
		msg         []byte
		expectError bool
	}{
		{
			[]byte(`{
				"method": "eth_subscribe",
				"params": ["newHeads"],
				"id": "abc"
			}`),
			false,
		},
		{
			[]byte(`{
				"method": "eth_subscribe",
				"params": ["newHeads"],
				"id": null
			}`),
			false,
		},
		{
			[]byte(`{
				"method": "eth_subscribe",
				"params": ["newHeads"],
				"id": 2.1
			}`),
			true,
		},
		{
			[]byte(`{
				"method": "eth_subscribe",
				"params": ["newHeads"]
			}`),
			false,
		},
		{
			[]byte(`{
				"method": "eth_subscribe",
				"params": ["newHeads"],
				"id": 2.0
			}`),
			false,
		},
	}
	for _, c := range cases {
		data, err := s.HandleWs(c.msg, mock)
		resp := new(SuccessResponse)
		merr := json.Unmarshal(data, resp)
		if merr != nil {
			t.Fatal("Invalid response")
		}
		if !c.expectError && (resp.Error != nil || err != nil) {
			t.Fatal("Error unexpected but found")
		}
		if c.expectError && (resp.Error == nil && err == nil) {
			t.Fatal("Error expected but not found")
		}
	}
}

type mockService struct {
	msgCh chan interface{}
}

func (m *mockService) Block(f BlockNumber) (interface{}, error) {
	m.msgCh <- f
	return nil, nil
}

func (m *mockService) Type(addr types.Address) (interface{}, error) {
	m.msgCh <- addr
	return nil, nil
}

func (m *mockService) BlockPtr(a string, f *BlockNumber) (interface{}, error) {
	if f == nil {
		m.msgCh <- nil
	} else {
		m.msgCh <- *f
	}
	return nil, nil
}

func (m *mockService) Filter(f LogFilter) (interface{}, error) {
	m.msgCh <- f
	return nil, nil
}

func TestDispatcherFuncDecode(t *testing.T) {
	srv := &mockService{msgCh: make(chan interface{}, 10)}

	s := newDispatcher(hclog.NewNullLogger(), newMockStore(), 0)
	s.registerService("mock", srv)

	handleReq := func(typ string, msg string) interface{} {
		_, err := s.handleReq(Request{
			Method: "mock_" + typ,
			Params: []byte(msg),
		})
		assert.NoError(t, err)
		return <-srv.msgCh
	}

	addr1 := types.Address{0x1}

	cases := []struct {
		typ string
		msg string
		res interface{}
	}{
		{
			"block",
			`["earliest"]`,
			EarliestBlockNumber,
		},
		{
			"block",
			`["latest"]`,
			LatestBlockNumber,
		},
		{
			"block",
			`["0x1"]`,
			BlockNumber(1),
		},
		{
			"type",
			`["` + addr1.String() + `"]`,
			addr1,
		},
		{
			"blockPtr",
			`["a"]`,
			nil,
		},
		{
			"blockPtr",
			`["a", "latest"]`,
			LatestBlockNumber,
		},
		{
			"filter",
			`[{"fromBlock": "pending", "toBlock": "earliest"}]`,
			LogFilter{fromBlock: PendingBlockNumber, toBlock: EarliestBlockNumber},
		},
	}
	for _, c := range cases {
		res := handleReq(c.typ, c.msg)
		if !reflect.DeepEqual(res, c.res) {
			t.Fatal("bad")
		}
	}
}

func TestDispatcherBatchRequest(t *testing.T) {
	s := newDispatcher(hclog.NewNullLogger(), newMockStore(), 0)
	s.registerEndpoints()

	// test with leading whitespace ("  \t\n\n\r")
	leftBytes := []byte{0x20, 0x20, 0x09, 0x0A, 0x0A, 0x0D}
	resp, err := s.Handle(append(leftBytes, []byte(`[
    {"id":1,"jsonrpc":"2.0","method":"eth_getBalance","params":["0x1", true]},
    {"id":2,"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["0x2", true]},
    {"id":3,"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["0x3", true]},
	{"id":4,"jsonrpc":"2.0","method": "web3_sha3","params": ["0x68656c6c6f20776f726c64"]}
]`)...))
	assert.NoError(t, err)

	var res []SuccessResponse
	assert.NoError(t, expectBatchJSONResult(resp, &res))
	assert.Len(t, res, 4)
	jsonerr := &ErrorObject{Code: -32602, Message: "Invalid Params"}
	assert.Equal(t, res[0].Error, jsonerr)
	assert.Nil(t, res[3].Error)
}

func TestDecodeTxn(t *testing.T) {
	tests := []struct {
		name     string
		accounts map[types.Address]*state.Account
		arg      *txnArgs
		res      *types.Transaction
		err      error
	}{
		{
			name: "should be failed when both Data and Input are given",
			arg: &txnArgs{
				From:     &addr1,
				To:       &addr2,
				Gas:      toArgUint64Ptr(21000),
				GasPrice: toArgBytesPtr(big.NewInt(10000).Bytes()),
				Value:    toArgBytesPtr(oneEther.Bytes()),
				Input:    toArgBytesPtr(big.NewInt(100).Bytes()),
				Data:     toArgBytesPtr(big.NewInt(200).Bytes()),
				Nonce:    toArgUint64Ptr(0),
			},
			res: nil,
			err: errors.New("both input and data cannot be set"),
		},
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
				Input:    nil,
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
				Input:    nil,
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
				Input:    nil,
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
				Input:    nil,
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
				Input:    nil,
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
		t.Run(tt.name, func(t *testing.T) {
			if tt.res != nil {
				tt.res.ComputeHash()
			}
			store := newMockStore()
			for addr, acc := range tt.accounts {
				store.SetAccount(addr, acc)
			}

			d := newTestDispatcher(hclog.NewNullLogger(), store)
			res, err := d.decodeTxn(tt.arg)
			assert.Equal(t, tt.res, res)
			assert.Equal(t, tt.err, err)
		})
	}
}

func TestDispatcher_GetNextNonce(t *testing.T) {
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

	dispatcher := newTestDispatcher(hclog.NewNullLogger(), store)

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
		t.Run(testCase.name, func(t *testing.T) {
			// Grab the nonce
			nonce, err := dispatcher.getNextNonce(testCase.account, testCase.number)

			// Assert errors
			assert.NoError(t, err)

			// Assert equality
			assert.Equal(t, testCase.expectedNonce, nonce)
		})
	}
}
