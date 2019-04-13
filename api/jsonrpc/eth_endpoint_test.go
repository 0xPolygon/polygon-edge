package jsonrpc

import (
	"testing"

	"github.com/umbracle/minimal/blockchain"
	"github.com/umbracle/minimal/minimal"
)

func newTestEthServer(minimal *minimal.Minimal) *Server {
	s := &Server{
		minimal:    minimal,
		serviceMap: map[string]*serviceData{},
	}
	s.endpoints.Eth = &Eth{s}
	s.registerService("eth", s.endpoints.Eth)
	return s
}

func TestEthEndpointGetBlockByNumber(t *testing.T) {
	headers := blockchain.NewTestHeaderChain(100)
	blockchain := blockchain.NewTestBlockchain(t, headers)

	minimal := &minimal.Minimal{
		Blockchain: blockchain,
	}
	s := newTestEthServer(minimal)

	resp, err := s.handle([]byte(`{
		"method": "eth_getBlockByNumber",
		"params": ["0x1", false]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	expectNonEmptyResult(t, resp)

	resp, err = s.handle([]byte(`{
		"method": "eth_getBlockByNumber",
		"params": ["0x11111", false]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	expectEmptyResult(t, resp)
}
