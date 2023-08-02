package jsonrpc

import (
	"encoding/json"
	"math/big"
	"reflect"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestDispatcher_HandleWebsocketConnection_EthSubscribe(t *testing.T) {
	t.Parallel()

	t.Run("clients should be able to receive \"newHeads\" event thru eth_subscribe", func(t *testing.T) {
		t.Parallel()

		store := newMockStore()
		dispatcher := newTestDispatcher(t,
			hclog.NewNullLogger(),
			store,
			&dispatcherParams{
				chainID:                 0,
				priceLimit:              0,
				jsonRPCBatchLengthLimit: 20,
				blockRangeLimit:         1000,
			},
		)
		mockConnection, msgCh := newMockWsConnWithMsgCh()

		req := []byte(`{
		"method": "eth_subscribe",
		"params": ["newHeads"]
	}`)
		if _, err := dispatcher.HandleWs(req, mockConnection); err != nil {
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
		case <-msgCh:
		case <-time.After(2 * time.Second):
			t.Fatal("\"newHeads\" event not received in 2 seconds")
		}
	})
}

func TestDispatcher_WebsocketConnection_RequestFormats(t *testing.T) {
	store := newMockStore()
	dispatcher := newTestDispatcher(t,
		hclog.NewNullLogger(),
		store,
		&dispatcherParams{
			chainID:                 0,
			priceLimit:              0,
			jsonRPCBatchLengthLimit: 20,
			blockRangeLimit:         1000,
		},
	)

	mockConnection, _ := newMockWsConnWithMsgCh()

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
		data, err := dispatcher.HandleWs(c.msg, mockConnection)
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

func (m *mockService) BlockPtr(_ string, f *BlockNumber) (interface{}, error) {
	if f == nil {
		m.msgCh <- nil
	} else {
		m.msgCh <- *f
	}

	return nil, nil
}

func (m *mockService) Filter(f LogQuery) (interface{}, error) {
	m.msgCh <- f

	return nil, nil
}

func TestDispatcherFuncDecode(t *testing.T) {
	srv := &mockService{msgCh: make(chan interface{}, 10)}

	dispatcher := newTestDispatcher(t,
		hclog.NewNullLogger(),
		newMockStore(),
		&dispatcherParams{
			chainID:                 0,
			priceLimit:              0,
			jsonRPCBatchLengthLimit: 20,
			blockRangeLimit:         1000,
		},
	)

	require.NoError(t, dispatcher.registerService("mock", srv))

	handleReq := func(typ string, msg string) interface{} {
		_, err := dispatcher.handleReq(Request{
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
			LogQuery{fromBlock: LatestBlockNumber, toBlock: EarliestBlockNumber}, // pending = latest
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
	handle := func(dispatcher *Dispatcher, reqBody []byte) []byte {
		res, _ := dispatcher.Handle(reqBody)

		return res
	}

	cases := []struct {
		name          string
		desc          string
		dispatcher    *Dispatcher
		reqBody       []byte
		err           *ObjectError
		batchResponse []*SuccessResponse
	}{
		{
			"leading-whitespace",
			"test with leading whitespace (\"  \\t\\n\\n\\r\\)",
			newTestDispatcher(t,
				hclog.NewNullLogger(),
				newMockStore(),
				&dispatcherParams{
					chainID:                 0,
					priceLimit:              0,
					jsonRPCBatchLengthLimit: 20,
					blockRangeLimit:         1000,
				},
			),
			append([]byte{0x20, 0x20, 0x09, 0x0A, 0x0A, 0x0D}, []byte(`[
				{"id":1,"jsonrpc":"2.0","method":"eth_getBalance","params":["0x1", true]},
				{"id":2,"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["0x2", true]},
				{"id":3,"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["0x3", true]},
				{"id":4,"jsonrpc":"2.0","method": "web3_sha3","params": ["0x68656c6c6f20776f726c64"]}]`)...),
			nil,
			[]*SuccessResponse{
				{Error: &ObjectError{Code: -32602, Message: "Invalid Params"}},
				{Error: nil},
				{Error: nil},
				{Error: nil}},
		},
		{
			"valid-batch-req",
			"test with batch req length within batchRequestLengthLimit",
			newTestDispatcher(t,
				hclog.NewNullLogger(),
				newMockStore(),
				&dispatcherParams{
					chainID:                 0,
					priceLimit:              0,
					jsonRPCBatchLengthLimit: 10,
					blockRangeLimit:         1000,
				},
			),
			[]byte(`[
				{"id":1,"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["latest", true]},
				{"id":2,"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["latest", true]},
				{"id":3,"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["latest", true]},
				{"id":4,"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["latest", true]},
				{"id":5,"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["latest", true]},
				{"id":6,"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["latest", true]}]`),
			nil,
			[]*SuccessResponse{
				{Error: nil},
				{Error: nil},
				{Error: nil},
				{Error: nil},
				{Error: nil},
				{Error: nil}},
		},
		{
			"invalid-batch-req",
			"test with batch req length exceeding batchRequestLengthLimit",
			newTestDispatcher(t,
				hclog.NewNullLogger(),
				newMockStore(),
				&dispatcherParams{
					chainID:                 0,
					priceLimit:              0,
					jsonRPCBatchLengthLimit: 3,
					blockRangeLimit:         1000,
				},
			),
			[]byte(`[
				{"id":1,"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["latest", true]},
				{"id":2,"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["latest", true]},
				{"id":3,"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["latest", true]},
				{"id":4,"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["latest", true]},
				{"id":5,"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["latest", true]},
				{"id":6,"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["latest", true]}]`),
			&ObjectError{Code: -32600, Message: "Batch request length too long"},
			nil,
		},
		{
			"no-limits",
			"test when limits are not set",
			newTestDispatcher(t,
				hclog.NewNullLogger(),
				newMockStore(),
				&dispatcherParams{
					chainID:                 0,
					priceLimit:              0,
					jsonRPCBatchLengthLimit: 0,
					blockRangeLimit:         0,
				}),
			[]byte(`[
				{"id":1,"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["latest", true]},
				{"id":2,"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["latest", true]},
				{"id":3,"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["latest", true]},
				{"id":4,"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["latest", true]},
				{"id":5,"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["latest", true]},
				{"id":6,"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["latest", true]},
				{"id":7,"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["latest", true]},
				{"id":8,"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["latest", true]},
				{"id":9,"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["latest", true]},
				{"id":10,"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["latest", true]},
				{"id":11,"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["latest", true]},
				{"id":12,"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["latest", true]}]`,
			),
			nil,
			[]*SuccessResponse{
				{Error: nil},
				{Error: nil},
				{Error: nil},
				{Error: nil},
				{Error: nil},
				{Error: nil},
				{Error: nil},
				{Error: nil},
				{Error: nil},
				{Error: nil},
				{Error: nil},
				{Error: nil},
			},
		},
	}

	for _, c := range cases {
		res := handle(c.dispatcher, c.reqBody)

		if c.err != nil {
			var resp ErrorResponse

			assert.NoError(t, expectBatchJSONResult(res, &resp))
			assert.Equal(t, resp.Error, c.err)
		} else {
			var batchResp []SuccessResponse
			assert.NoError(t, expectBatchJSONResult(res, &batchResp))

			if c.name == "leading-whitespace" {
				assert.Len(t, batchResp, 4)
				for index, resp := range batchResp {
					assert.Equal(t, resp.Error, c.batchResponse[index].Error)
				}
			} else if c.name == "valid-batch-req" {
				assert.Len(t, batchResp, 6)
				for index, resp := range batchResp {
					assert.Equal(t, resp.Error, c.batchResponse[index].Error)
				}
			} else if c.name == "no-limits" {
				assert.Len(t, batchResp, 12)
				for index, resp := range batchResp {
					assert.Equal(t, resp.Error, c.batchResponse[index].Error)
				}
			}
		}
	}
}

func newTestDispatcher(t *testing.T, logger hclog.Logger, store JSONRPCStore, params *dispatcherParams) *Dispatcher {
	t.Helper()

	d, err := newDispatcher(logger, store, params)
	require.NoError(t, err)

	return d
}
