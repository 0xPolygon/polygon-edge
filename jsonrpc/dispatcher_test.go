package jsonrpc

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/0xPolygon/minimal/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
)

func expectJSONResult(data []byte, v interface{}) error {
	var resp Response
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
		_, err := s.HandleWs(c.msg, mock)
		if !c.expectError && err != nil {
			t.Fatal("Error unexpected but found")
		}
		if c.expectError && err == nil {
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
    {"id":1,"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["0x1", true]},
    {"id":2,"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["0x2", true]},
    {"id":3,"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["0x3", true]},
		{"id":4,"jsonrpc":"2.0","method": "web3_sha3","params": ["0x68656c6c6f20776f726c64"]}
]`)...))
	assert.NoError(t, err)

	var res []Response
	assert.NoError(t, expectBatchJSONResult(resp, &res))
	assert.Len(t, res, 4)
	assert.Equal(t, res[0].Error, internalError)
	assert.Nil(t, res[3].Error)
}
