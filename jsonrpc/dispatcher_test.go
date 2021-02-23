package jsonrpc

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/0xPolygon/minimal/types"
	"github.com/hashicorp/go-hclog"
)

func expectEmptyResult(t *testing.T, data []byte) {
	var i interface{}
	if err := expectJSONResult(data, &i); err != nil {
		t.Fatal(err)
	}
	if i != nil {
		t.Fatal("expected empty result")
	}
}

func expectNonEmptyResult(t *testing.T, data []byte) {
	var i interface{}
	if err := expectJSONResult(data, &i); err != nil {
		t.Fatal(err)
	}
	if i == nil {
		t.Fatal("expected non empty result")
	}
}

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

	s := newDispatcher(hclog.NewNullLogger(), store)
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
