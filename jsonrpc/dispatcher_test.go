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
