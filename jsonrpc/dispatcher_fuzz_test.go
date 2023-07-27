package jsonrpc

import (
	"testing"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
)

func FuzzDispatcherFuncDecode(f *testing.F) {
	srv := &mockService{msgCh: make(chan interface{}, 10)}

	dispatcher := newTestDispatcher(f,
		hclog.NewNullLogger(),
		newMockStore(),
		&dispatcherParams{
			chainID:                 0,
			priceLimit:              0,
			jsonRPCBatchLengthLimit: 20,
			blockRangeLimit:         1000,
		},
	)

	require.NoError(f, dispatcher.registerService("mock", srv))

	handleReq := func(typ string, msg string) interface{} {
		_, err := dispatcher.handleReq(Request{
			Method: "mock_" + typ,
			Params: []byte(msg),
		})
		if err != nil {
			return err
		}

		return <-srv.msgCh
	}

	addr1 := types.Address{0x1}

	seeds := []struct {
		typ string
		msg string
	}{
		{
			"block",
			`["earliest"]`,
		},
		{
			"block",
			`["latest"]`,
		},
		{
			"block",
			`["0x1"]`,
		},
		{
			"type",
			`["` + addr1.String() + `"]`,
		},
		{
			"blockPtr",
			`["a"]`,
		},
		{
			"blockPtr",
			`["a", "latest"]`,
		},
		{
			"filter",
			`[{"fromBlock": "pending", "toBlock": "earliest"}]`,
		},
		{
			"block",
			"[\"8\"]",
		},
		{
			"block",
			"[\"009\"]",
		},
		{
			"block",
			"10",
		},
	}

	for _, seed := range seeds {
		f.Add(seed.typ, seed.msg)
	}

	f.Fuzz(func(t *testing.T, typ string, msg string) {
		handleReq(typ, msg)
	})
}

func FuzzDispatcherBatchRequest(f *testing.F) {
	mock := &mockWsConn{
		SetFilterIDFn: func(s string) {
		},
		GetFilterIDFn: func() string {
			return ""
		},
		WriteMessageFn: func(i int, b []byte) error {
			return nil
		},
	}

	seeds := []struct {
		batchLimit uint64
		blockLimit uint64
		body       string
	}{
		{
			batchLimit: 10,
			blockLimit: 1000,
			body: `[
				{"id":1,"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["latest", true]},
				{"id":2,"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["latest", true]},
				{"id":3,"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["latest", true]},
				{"id":4,"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["latest", true]},
				{"id":5,"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["latest", true]},
				{"id":6,"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["latest", true]}]`,
		},
		{
			batchLimit: 3,
			blockLimit: 1000,
			body: `[
				{"id":1,"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["latest", true]},
				{"id":2,"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["latest", true]},
				{"id":3,"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["latest", true]},
				{"id":4,"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["latest", true]},
				{"id":5,"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["latest", true]},
				{"id":6,"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["latest", true]}]`,
		},
		{
			batchLimit: 0,
			blockLimit: 0,
			body: `[
				{"id":1,"jsonrpc":"2.0","method":"eth_getBalance","params":["0x1", true]},
				{"id":2,"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["0x2", true]},
				{"id":3,"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["0x3", true]},
				{"id":4,"jsonrpc":"2.0","method": "web3_sha3","params": ["0x68656c6c6f20776f726c64"]}]`,
		},
		{
			batchLimit: 0,
			blockLimit: 0,
			body: `[
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
		},
	}

	for _, seed := range seeds {
		f.Add(seed.batchLimit, seed.blockLimit, seed.body)
	}

	f.Fuzz(func(t *testing.T, batchLimit uint64, blockLimit uint64, body string) {
		dispatcher := newTestDispatcher(t,
			hclog.NewNullLogger(),
			newMockStore(),
			&dispatcherParams{
				chainID:                 0,
				priceLimit:              0,
				jsonRPCBatchLengthLimit: batchLimit,
				blockRangeLimit:         blockLimit,
			},
		)

		_, _ = dispatcher.HandleWs([]byte(body), mock)
		_, _ = dispatcher.Handle([]byte(body))
	})
}

func FuzzDispatcherWebsocketConnectionUnsubscribe(f *testing.F) {
	store := newMockStore()
	dispatcher := newTestDispatcher(f,
		hclog.NewNullLogger(),
		store,
		&dispatcherParams{
			chainID:                 0,
			priceLimit:              0,
			jsonRPCBatchLengthLimit: 20,
			blockRangeLimit:         1000,
		},
	)
	mockConn := &mockWsConn{
		SetFilterIDFn: func(s string) {
		},
		GetFilterIDFn: func() string {
			return ""
		},
		WriteMessageFn: func(i int, b []byte) error {
			return nil
		},
	}

	seeds := []string{
		`{"method": "eth_unsubscribe", "params": ["787832"]}`,
		`{"method": "eth_subscribe", "params": ["newHeads"]}`,
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, request string) {
		_, _ = dispatcher.HandleWs([]byte(request), mockConn)
	})
}
