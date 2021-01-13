package jsonrpc

import (
	"testing"

	"github.com/0xPolygon/minimal/blockchain"
	"github.com/0xPolygon/minimal/minimal"
)

func TestEthEndpointGetBlockByNumber(t *testing.T) {
	headers := blockchain.NewTestHeaderChain(100)
	blockchain := blockchain.NewTestBlockchain(t, headers)

	minimal := &minimal.Minimal{
		Blockchain: blockchain,
	}
	s := newTestDispatcher("eth")
	s.minimal = minimal

	resp, err := s.handle(serverHTTP, []byte(`{
		"method": "eth_getBlockByNumber",
		"params": ["0x1", false]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	expectNonEmptyResult(t, resp)

	resp, err = s.handle(serverHTTP, []byte(`{
		"method": "eth_getBlockByNumber",
		"params": ["0x11111", false]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	expectEmptyResult(t, resp)
}

// TODO: Test filterLog rpc endpoint decoding
