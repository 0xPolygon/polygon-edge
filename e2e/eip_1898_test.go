package e2e

import (
	"github.com/0xPolygon/polygon-sdk/e2e/framework"
	"github.com/0xPolygon/polygon-sdk/helper/tests"
	"github.com/0xPolygon/polygon-sdk/jsonrpc"
	"github.com/0xPolygon/polygon-sdk/types"
	"github.com/stretchr/testify/assert"
	"github.com/umbracle/go-web3"
	"math/big"
	"testing"
)

func Test_GetBalance(t *testing.T) {
	// Test account
	_, address := tests.GenerateKeyAndAddr(t)

	// Network
	servers := framework.NewTestServers(t, 1, func(config *framework.TestServerConfig) {
		config.SetConsensus(framework.ConsensusDev)
		config.Premine(address, framework.EthToWei(1))
	})

	// Client
	srv := servers[0]
	client := srv.JSONRPC()

	expected := framework.EthToWei(1)

	// Using web3
	balance, err := client.Eth().GetBalance(web3.Address(address), web3.Latest)
	assert.NoError(t, err)
	assert.Equal(t, expected, balance)

	var out string

	// Using custom number
	err = client.Call("eth_getBalance", &out, address, "0x0")
	assert.NoError(t, err)
	balance, ok := new(big.Int).SetString(out[2:], 16)
	assert.Equal(t, true, ok)
	assert.Equal(t, expected, balance)

	// Using implicit latest #1
	err = client.Call("eth_getBalance", &out, address)
	assert.NoError(t, err)
	balance, ok = new(big.Int).SetString(out[2:], 16)
	assert.Equal(t, true, ok)
	assert.Equal(t, expected, balance)

	// Using implicit latest #2
	err = client.Call("eth_getBalance", &out, address, nil)
	assert.NoError(t, err)
	balance, ok = new(big.Int).SetString(out[2:], 16)
	assert.Equal(t, true, ok)
	assert.Equal(t, expected, balance)

	// Using number as an object
	n := jsonrpc.BlockNumber(0)
	params := jsonrpc.BlockNumberOrHash{
		BlockNumber: &n,
	}
	err = client.Call("eth_getBalance", &out, address, params)
	assert.NoError(t, err)
	balance, ok = new(big.Int).SetString(out[2:], 16)
	assert.Equal(t, true, ok)
	assert.Equal(t, expected, balance)

	// Using block hash as an object
	block, err := client.Eth().GetBlockByNumber(web3.Latest, false)
	assert.NoError(t, err)

	params = jsonrpc.BlockNumberOrHash{
		BlockNumber: nil,
		BlockHash:   (*types.Hash)(&block.Hash),
	}
	err = client.Call("eth_getBalance", &out, address, params)
	assert.NoError(t, err)
	balance, ok = new(big.Int).SetString(out[2:], 16)
	assert.Equal(t, true, ok)
	assert.Equal(t, expected, balance)

	// Using invalid number
	err = client.Call("eth_getBalance", &out, address, "abc")
	assert.Error(t, err)
}
