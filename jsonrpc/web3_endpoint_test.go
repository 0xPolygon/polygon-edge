package jsonrpc

import (
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
)

func TestWeb3EndpointSha3(t *testing.T) {
	s := newDispatcher(hclog.NewNullLogger(), newMockStore(), 0)
	s.registerEndpoints()

	resp, err := s.Handle([]byte(`{
		"method": "web3_sha3",
		"params": ["0x68656c6c6f20776f726c64"]
	}`))
	assert.NoError(t, err)

	var res string
	assert.NoError(t, expectJSONResult(resp, &res))
	assert.Equal(t, res, "0x47173285a8d7341e5e972fc677286384f802f8ef42a5ec5f03bbfa254cb01fad")
}
