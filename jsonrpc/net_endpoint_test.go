package jsonrpc

import (
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
)

func TestNetEndpoint_PeerCount(t *testing.T) {
	dispatcher := newTestDispatcher(t,
		hclog.NewNullLogger(),
		newMockStore(),
		&dispatcherParams{
			chainID: 1,
		})

	resp, err := dispatcher.Handle([]byte(`{
		"method": "net_peerCount",
		"params": [""]
	}`))
	assert.NoError(t, err)

	var res string

	assert.NoError(t, expectJSONResult(resp, &res))
	assert.Equal(t, "0x14", res)
}
