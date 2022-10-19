package jsonrpc

import (
	"fmt"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
	"go.uber.org/goleak"

	"github.com/0xPolygon/polygon-edge/versioning"
)

func TestWeb3EndpointSha3(t *testing.T) {
	defer goleak.VerifyNone(t)

	dispatcher := newDispatcher(
		hclog.NewNullLogger(),
		newMockStore(),
		&dispatcherParams{
			chainID:                 0,
			priceLimit:              0,
			jsonRPCBatchLengthLimit: 20,
			blockRangeLimit:         1000,
		})

	resp, err := dispatcher.Handle([]byte(`{
		"method": "web3_sha3",
		"params": ["0x68656c6c6f20776f726c64"]
	}`))
	assert.NoError(t, err)

	var res string

	assert.NoError(t, expectJSONResult(resp, &res))
	assert.Equal(t, "0x47173285a8d7341e5e972fc677286384f802f8ef42a5ec5f03bbfa254cb01fad", res)
}

func TestWeb3EndpointClientVersion(t *testing.T) {
	defer goleak.VerifyNone(t)

	var (
		chainName = "test-chain"
		chainID   = uint64(100)
	)

	dispatcher := newDispatcher(
		hclog.NewNullLogger(),
		newMockStore(),
		&dispatcherParams{
			chainID:                 chainID,
			chainName:               chainName,
			priceLimit:              0,
			jsonRPCBatchLengthLimit: 20,
			blockRangeLimit:         1000,
		},
	)

	resp, err := dispatcher.Handle([]byte(`{
		"method": "web3_clientVersion",
		"params": []
	}`))
	assert.NoError(t, err)

	var res string

	assert.NoError(t, expectJSONResult(resp, &res))
	assert.Contains(t, res,
		fmt.Sprintf(
			clientVersionTemplate,
			chainName,
			chainID,
			versioning.Version,
		),
	)
}
