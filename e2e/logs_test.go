package e2e

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-sdk/e2e/framework"
	"github.com/0xPolygon/polygon-sdk/helper/tests"
	"github.com/stretchr/testify/assert"
	"github.com/umbracle/go-web3"
)

func TestNewFilter_Logs(t *testing.T) {
	_, addr := tests.GenerateKeyAndAddr(t)
	srvs := framework.NewTestServers(t, 1, func(config *framework.TestServerConfig) {
		config.SetConsensus(framework.ConsensusDev)
		config.Premine(addr, framework.EthToWei(10))
		config.SetSeal(true)
	})
	srv := srvs[0]

	ctx1, cancel1 := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel1()
	contractAddr, err := srv.DeployContract(ctx1, sampleByteCode)
	if err != nil {
		t.Fatal(err)
	}

	client := srv.JSONRPC()
	id, err := client.Eth().NewFilter(&web3.LogFilter{})
	assert.NoError(t, err)

	numCalls := 10
	for i := 0; i < numCalls; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		srv.TxnTo(ctx, contractAddr, "setA1")
	}

	res, err := client.Eth().GetFilterChanges(id)
	assert.NoError(t, err)
	assert.Equal(t, len(res), numCalls)
}

func TestNewFilter_Block(t *testing.T) {
	_, from := tests.GenerateKeyAndAddr(t)
	_, to := tests.GenerateKeyAndAddr(t)
	toAddr := web3.HexToAddress(to.String())

	srvs := framework.NewTestServers(t, 1, func(config *framework.TestServerConfig) {
		config.SetConsensus(framework.ConsensusDev)
		config.Premine(from, framework.EthToWei(10))
		config.SetSeal(true)
	})
	srv := srvs[0]
	client := srv.JSONRPC()

	id, err := client.Eth().NewBlockFilter()
	assert.NoError(t, err)

	for i := 0; i < 3; i++ {
		hash, err := client.Eth().SendTransaction(&web3.Transaction{
			From:     web3.HexToAddress(srv.Config.PremineAccts[0].Addr.String()),
			To:       &toAddr,
			GasPrice: 10000,
			Gas:      1000000,
			Value:    big.NewInt(10000),
			Nonce:    uint64(i),
		})
		assert.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, err = srv.WaitForReceipt(ctx, hash)
		assert.NoError(t, err)
	}

	// there should be three changes
	blocks, err := client.Eth().GetFilterChangesBlock(id)
	assert.NoError(t, err)
	assert.Len(t, blocks, 3)
}
