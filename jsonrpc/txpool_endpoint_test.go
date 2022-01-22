package jsonrpc

import (
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
)

func TestContentEndpoint(t *testing.T) {
	dispatcher := newDispatcher(hclog.NewNullLogger(), newMockStore(), 0)

	resp, err := dispatcher.Handle([]byte(`{
		"method": "txpool_content",
		"params": []
	}`))
	assert.NoError(t, err)

	var res ContentResponse

	assert.NoError(t, expectJSONResult(resp, &res))
	assert.Len(t, res.Pending, 0)
	assert.Len(t, res.Queued, 0)
}

func TestInspectEndpoint(t *testing.T) {
	dispatcher := newDispatcher(hclog.NewNullLogger(), newMockStore(), 0)

	resp, err := dispatcher.Handle([]byte(`{
		"method": "txpool_inspect",
		"params": []
	}`))
	assert.NoError(t, err)

	var res InspectResponse

	assert.NoError(t, expectJSONResult(resp, &res))
	assert.Len(t, res.Pending, 0)
	assert.Len(t, res.Queued, 0)
	assert.Equal(t, res.CurrentCapacity, uint64(0))
	assert.Equal(t, res.MaxCapacity, uint64(0))
}

func TestStatusEndpoint(t *testing.T) {
	dispatcher := newDispatcher(hclog.NewNullLogger(), newMockStore(), 0)

	resp, err := dispatcher.Handle([]byte(`{
		"method": "txpool_status",
		"params": []
	}`))
	assert.NoError(t, err)

	var res StatusResponse

	assert.NoError(t, expectJSONResult(resp, &res))
	assert.Equal(t, res.Pending, uint64(0))
	assert.Equal(t, res.Queued, uint64(0))
}
