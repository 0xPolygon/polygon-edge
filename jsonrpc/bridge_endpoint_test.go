package jsonrpc

import (
	"encoding/json"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
)

func TestBridgeEndpoint(t *testing.T) {
	store := newMockStore()

	dispatcher := newTestDispatcher(t,
		hclog.NewNullLogger(),
		store,
		&dispatcherParams{
			chainID:                 0,
			priceLimit:              0,
			jsonRPCBatchLengthLimit: 20,
			blockRangeLimit:         1000,
		},
	)

	mockConnection, _ := newMockWsConnWithMsgCh()

	msg := []byte(`{
		"method": "bridge_generateExitProof",
		"params": ["0x0001"],
		"id": 1
	}`)

	data, err := dispatcher.HandleWs(msg, mockConnection)
	require.NoError(t, err)

	resp := new(SuccessResponse)
	require.NoError(t, json.Unmarshal(data, resp))
	require.Nil(t, resp.Error)
	require.NotNil(t, resp.Result)

	msg = []byte(`{
		"method": "bridge_getStateSyncProof",
		"params": ["0x1"],
		"id": 1
	}`)

	data, err = dispatcher.HandleWs(msg, mockConnection)
	require.NoError(t, err)

	resp = new(SuccessResponse)
	require.NoError(t, json.Unmarshal(data, resp))
	require.Nil(t, resp.Error)
	require.NotNil(t, resp.Result)
}
