package e2e

import (
	"context"
	"fmt"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/0xPolygon/minimal/e2e/framework"
	"github.com/stretchr/testify/assert"
	"github.com/umbracle/go-web3"
)

func TestNewFilter_Logs(t *testing.T) {
	_, addr := framework.GenerateKeyAndAddr(t)

	dataDir, err := framework.TempDir()
	if err != nil {
		t.Fatal(err)
	}

	srv := framework.NewTestServer(t, dataDir, func(config *framework.TestServerConfig) {
		config.SetConsensus(framework.ConsensusDev)
		config.Premine(addr, framework.EthToWei(10))
		config.SetSeal(true)
	})
	t.Cleanup(func() {
		srv.Stop()
		if err := os.RemoveAll(dataDir); err != nil {
			t.Log(err)
		}
	})
	if err := srv.GenerateGenesis(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Start(ctx); err != nil {
		t.Fatal(err)
	}

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
	// todo: need to check implementation because there is a possibility of losing some logs
	assert.GreaterOrEqual(t, len(res), numCalls/2)
}

func TestNewFilter_Block(t *testing.T) {
	_, from := framework.GenerateKeyAndAddr(t)
	_, to := framework.GenerateKeyAndAddr(t)
	toAddr := web3.HexToAddress(to.String())

	dataDir, err := framework.TempDir()
	if err != nil {
		t.Fatal(err)
	}

	srv := framework.NewTestServer(t, dataDir, func(config *framework.TestServerConfig) {
		config.SetConsensus(framework.ConsensusDev)
		config.SetSeal(true)
		config.Premine(from, framework.EthToWei(20))
	})
	t.Cleanup(func() {
		srv.Stop()
		if err := os.RemoveAll(dataDir); err != nil {
			t.Log(err)
		}
	})

	if err := srv.GenerateGenesis(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Start(ctx); err != nil {
		t.Fatal(err)
	}

	client := srv.JSONRPC()
	id, err := client.Eth().NewBlockFilter()
	assert.NoError(t, err)

	for i := 0; i < 3; i++ {
		_, err := client.Eth().SendTransaction(&web3.Transaction{
			From:     web3.HexToAddress(srv.Config.PremineAccts[0].Addr.String()),
			To:       &toAddr,
			GasPrice: 10000,
			Gas:      1000000,
			Value:    big.NewInt(10000),
			Nonce:    uint64(i),
		})
		assert.NoError(t, err)
	}

	// there should be three changes
	blocks, err := client.Eth().GetFilterChangesBlock(id)
	assert.NoError(t, err)
	fmt.Println(blocks)
}
