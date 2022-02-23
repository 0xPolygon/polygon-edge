package e2e

import (
	"context"
	"github.com/0xPolygon/polygon-edge/types"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/e2e/framework"
	"github.com/0xPolygon/polygon-edge/helper/tests"
	"github.com/stretchr/testify/assert"
)

func TestEncoding(t *testing.T) {
	key, from := tests.GenerateKeyAndAddr(t)

	srvs := framework.NewTestServers(t, 1, func(config *framework.TestServerConfig) {
		config.SetConsensus(framework.ConsensusDev)
		config.SetSeal(true)
		config.Premine(from, framework.EthToWei(10))
	})
	srv := srvs[0]

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	contractAddr, err := srv.DeployContract(ctx, sampleByteCode, key)

	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	receipt := srv.InvokeMethod(ctx, types.Address(contractAddr), "setA1", key)

	// try to get the transaction
	client := srv.JSONRPC().Eth()

	_, err = client.GetTransactionByHash(receipt.TransactionHash)
	assert.NoError(t, err)

	_, err = client.GetBlockByHash(receipt.BlockHash, true)
	assert.NoError(t, err)
}
