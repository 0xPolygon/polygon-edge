package e2e

import (
	"os"
	"testing"

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
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}

	contractAddr, err := srv.DeployContract(sampleByteCode)
	if err != nil {
		t.Fatal(err)
	}

	// send a transaction
	receipt := srv.TxnTo(contractAddr, "setA1")

	// try to get the transaction
	client := srv.JSONRPC().Eth()

	_, err = client.GetTransactionByHash(receipt.TransactionHash)
	assert.NoError(t, err)

	_, err = client.GetBlockByHash(receipt.BlockHash, true)
	assert.NoError(t, err)
}
