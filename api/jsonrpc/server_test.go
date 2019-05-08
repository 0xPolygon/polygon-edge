package jsonrpc

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func newTestServer(endpoints ...string) *Server {
	s := newServer()
	s.registerEndpoints()
	s.enableEndpoints(serverHTTP, endpoints)
	return s
}

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

func TestServerEnableEndpoints(t *testing.T) {
	s := newServer()
	s.registerEndpoints()

	req := []byte(`{
		"method": "web3_sha3",
		"params": ["0x68656c6c6f20776f726c64"]
	}`)

	validate := func(typ serverType) {
		_, err := s.handle(typ, req)
		assert.Error(t, err)

		s.enableEndpoints(typ, []string{"web3"})

		_, err = s.handle(typ, req)
		assert.NoError(t, err)

		s.disableEndpoints(typ, []string{"web3"})

		_, err = s.handle(typ, req)
		assert.Error(t, err)
	}

	validate(serverHTTP)
	validate(serverIPC)
	validate(serverWS)
}
