package jsonrpc

import (
	"testing"
)

func newTestWeb3Server() *Server {
	s := &Server{
		serviceMap: map[string]*serviceData{},
	}
	s.endpoints.Web3 = &Web3{s}
	s.registerService("web3", s.endpoints.Web3)
	return s
}

func TestWeb3EndpointSha3(t *testing.T) {
	s := newTestWeb3Server()

	resp, err := s.handle([]byte(`{
		"method": "web3_sha3",
		"params": ["0x68656c6c6f20776f726c64"]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	var res string
	if err := expectJSONResult(resp, &res); err != nil {
		t.Fatal(err)
	}
	if res != "0x47173285a8d7341e5e972fc677286384f802f8ef42a5ec5f03bbfa254cb01fad" {
		t.Fatal("bad")
	}
}
