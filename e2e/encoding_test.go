package e2e

import (
	"testing"

	"github.com/0xPolygon/minimal/e2e/framework"
	"github.com/0xPolygon/minimal/types"
	"github.com/stretchr/testify/assert"
)

func TestEncoding(t *testing.T) {
	dataDir, err := framework.TempDir()
	if err != nil {
		t.Fatal(err)
	}

	srv := framework.NewTestServer(t, dataDir, func(config *framework.TestServerConfig) {
		config.SetDev(true)
		config.SetSeal(true)
		config.SetConsensus(framework.ConsensusDummy)
		config.SetShowsLog(true)
		config.Premine(types.StringToAddress("0xdf7fd4830f4cc1440b469615e9996e9fde92608f"), framework.EthToWei(10))
	})
	t.Cleanup(func() {
		srv.Stop()
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
