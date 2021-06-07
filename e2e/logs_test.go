package e2e

import (
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
		config.Premine(addr, framework.EthToWei(20))
		config.SetSeal(true)
		config.SetShowsLog(true)
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
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}

	contractAddr, err := srv.DeployContract(sampleByteCode)
	if err != nil {
		t.Fatal(err)
	}

	client := srv.JSONRPC()
	id, err := client.Eth().NewFilter(&web3.LogFilter{})
	assert.NoError(t, err)

	numCalls := 10
	for i := 0; i < numCalls; i++ {
		srv.TxnTo(contractAddr, "setA1")
	}
	time.Sleep(5 * time.Second)

	res, err := client.Eth().GetFilterChanges(id)
	assert.NoError(t, err)
	assert.Len(t, res, numCalls)
}

func TestNewFilter_Block(t *testing.T) {
	_, from := framework.GenerateKeyAndAddr(t)
	target := web3.HexToAddress("0x1010101010101010101010101010101010101010")

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
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}

	client := srv.JSONRPC()
	id, err := client.Eth().NewBlockFilter()
	assert.NoError(t, err)

	for i := 0; i < 3; i++ {
		_, err := client.Eth().SendTransaction(&web3.Transaction{
			From:     web3.HexToAddress(srv.Config.PremineAccts[0].Addr.String()),
			To:       &target,
			GasPrice: 10000,
			Gas:      1000000,
			Value:    big.NewInt(10000),
			Nonce:    uint64(i),
		})
		assert.NoError(t, err)
	}
	time.Sleep(5 * time.Second)

	// there should be three changes
	blocks, err := client.Eth().GetFilterChangesBlock(id)
	assert.NoError(t, err)
	fmt.Println(blocks)
}
