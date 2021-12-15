package e2e

import (
	"context"
	"github.com/0xPolygon/polygon-sdk/e2e/framework"
	"github.com/0xPolygon/polygon-sdk/helper/tests"
	"github.com/0xPolygon/polygon-sdk/jsonrpc"
	"github.com/0xPolygon/polygon-sdk/types"
	"github.com/stretchr/testify/assert"
	"github.com/umbracle/go-web3"
	"math/big"
	"testing"
	"time"
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

func Test_GetTransactionCount(t *testing.T) {
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

	expected := uint64(0)

	// Using web3
	nonce, err := client.Eth().GetNonce(web3.Address(address), web3.Latest)
	assert.NoError(t, err)
	assert.Equal(t, expected, nonce)

	var out string

	// Using custom number
	err = client.Call("eth_getTransactionCount", &out, address, "0x0")
	assert.NoError(t, err)
	nonce, err = types.ParseUint64orHex(&out)
	assert.NoError(t, err)
	assert.Equal(t, expected, nonce)

	// Using implicit latest #1
	err = client.Call("eth_getTransactionCount", &out, address)
	assert.NoError(t, err)
	nonce, err = types.ParseUint64orHex(&out)
	assert.NoError(t, err)
	assert.Equal(t, expected, nonce)

	// Using implicit latest #2
	err = client.Call("eth_getTransactionCount", &out, address, nil)
	assert.NoError(t, err)
	nonce, err = types.ParseUint64orHex(&out)
	assert.NoError(t, err)
	assert.Equal(t, expected, nonce)

	// Using number as an object
	n := jsonrpc.BlockNumber(0)
	params := jsonrpc.BlockNumberOrHash{
		BlockNumber: &n,
	}
	err = client.Call("eth_getTransactionCount", &out, address, params)
	assert.NoError(t, err)
	nonce, err = types.ParseUint64orHex(&out)
	assert.NoError(t, err)
	assert.Equal(t, expected, nonce)

	// Using block hash as an object
	block, err := client.Eth().GetBlockByNumber(web3.Latest, false)
	assert.NoError(t, err)

	params = jsonrpc.BlockNumberOrHash{
		BlockNumber: nil,
		BlockHash:   (*types.Hash)(&block.Hash),
	}
	err = client.Call("eth_getTransactionCount", &out, address, params)
	assert.NoError(t, err)
	nonce, err = types.ParseUint64orHex(&out)
	assert.NoError(t, err)
	assert.Equal(t, expected, nonce)

	// Using invalid number
	err = client.Call("eth_getTransactionCount", &out, address, "abc")
	assert.Error(t, err)
}

func Test_GetCode(t *testing.T) {
	// Test account
	_, address := tests.GenerateKeyAndAddr(t)

	// Network
	servers := framework.NewTestServers(t, 1, func(config *framework.TestServerConfig) {
		config.SetConsensus(framework.ConsensusDev)
		config.Premine(address, framework.EthToWei(1))
	})

	// Client
	srv := servers[0]

	ctx1, cancel1 := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel1()
	contractAddr, err := srv.DeployContract(ctx1, sampleByteCode)
	if err != nil {
		t.Fatal(err)
	}

	client := srv.JSONRPC()

	expected := "0x6080604052600436106100345760003560e01c8063498e6857146100395780636e7e996e14610043578063dd13c6171461004d575b600080fd5b610041610069565b005b61004b6100da565b005b610067600480360361006291908101906101be565b61014b565b005b600073ffffffffffffffffffffffffffffffffffffffff1673010000000000000000000000000000000000000073ffffffffffffffffffffffffffffffffffffffff167fec89f5028137f6e2218f9178e2ddfa454a509f4778d9cf323e96f42a902d307f60405160405180910390a3565b73010000000000000000000000000000000000000073ffffffffffffffffffffffffffffffffffffffff16600073ffffffffffffffffffffffffffffffffffffffff167fec89f5028137f6e2218f9178e2ddfa454a509f4778d9cf323e96f42a902d307f60405160405180910390a3565b8073ffffffffffffffffffffffffffffffffffffffff168273ffffffffffffffffffffffffffffffffffffffff167fec89f5028137f6e2218f9178e2ddfa454a509f4778d9cf323e96f42a902d307f60405160405180910390a35050565b6000813590506101b88161022c565b92915050565b600080604083850312156101d157600080fd5b60006101df858286016101a9565b92505060206101f0858286016101a9565b9150509250929050565b60006102058261020c565b9050919050565b600073ffffffffffffffffffffffffffffffffffffffff82169050919050565b610235816101fa565b811461024057600080fd5b5056fea365627a7a72315820874667b781b97cc25ada056a6b1d529cadb5d6e734754990b055e2d072d506d16c6578706572696d656e74616cf564736f6c63430005110040"

	// Using web3
	code, err := client.Eth().GetCode(contractAddr, web3.Latest)
	assert.NoError(t, err)
	assert.Equal(t, expected, code)

	var out string

	// Using custom number
	err = client.Call("eth_getCode", &out, contractAddr, "0x1")
	assert.NoError(t, err)
	assert.NoError(t, err)
	assert.Equal(t, expected, out)

	// Using implicit latest #1
	err = client.Call("eth_getCode", &out, contractAddr)
	assert.NoError(t, err)
	assert.NoError(t, err)
	assert.Equal(t, expected, out)

	// Using implicit latest #2
	err = client.Call("eth_getCode", &out, contractAddr, nil)
	assert.NoError(t, err)
	assert.NoError(t, err)
	assert.Equal(t, expected, out)

	// Using number as an object
	n := jsonrpc.BlockNumber(1)
	params := jsonrpc.BlockNumberOrHash{
		BlockNumber: &n,
	}
	err = client.Call("eth_getCode", &out, contractAddr, params)
	assert.NoError(t, err)
	assert.NoError(t, err)
	assert.Equal(t, expected, out)

	// Using block hash as an object
	block, err := client.Eth().GetBlockByNumber(web3.Latest, false)
	assert.NoError(t, err)

	params = jsonrpc.BlockNumberOrHash{
		BlockNumber: nil,
		BlockHash:   (*types.Hash)(&block.Hash),
	}
	err = client.Call("eth_getCode", &out, contractAddr, params)
	assert.NoError(t, err)
	assert.NoError(t, err)
	assert.Equal(t, expected, out)

	// Using invalid number
	err = client.Call("eth_getCode", &out, address, "abc")
	assert.Error(t, err)
}
