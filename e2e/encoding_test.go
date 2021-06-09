package e2e

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/0xPolygon/minimal/e2e/framework"
	"github.com/stretchr/testify/assert"
)

func TestEncoding(t *testing.T) {
	_, from := framework.GenerateKeyAndAddr(t)

	dataDir, err := framework.TempDir()
	if err != nil {
		t.Fatal(err)
	}

	srv := framework.NewTestServer(t, dataDir, func(config *framework.TestServerConfig) {
		config.SetConsensus(framework.ConsensusDev)
		config.SetSeal(true)
		config.Premine(from, framework.EthToWei(10))
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

	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	contractAddr, err := srv.DeployContract(ctx, sampleByteCode)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	receipt := srv.TxnTo(ctx, contractAddr, "setA1")

	// try to get the transaction
	client := srv.JSONRPC().Eth()

	_, err = client.GetTransactionByHash(receipt.TransactionHash)
	assert.NoError(t, err)

	_, err = client.GetBlockByHash(receipt.BlockHash, true)
	assert.NoError(t, err)
}
