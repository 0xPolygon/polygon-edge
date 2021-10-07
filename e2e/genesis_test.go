package e2e

import (
	"github.com/0xPolygon/polygon-sdk/command/helper"
	"github.com/0xPolygon/polygon-sdk/e2e/framework"
	"testing"
)

func TestGenesisCustomBlockGasLimit(t *testing.T) {
	var blockGasLimit uint64 = 5000000000
	srvs := framework.NewTestServers(t, 1, func(config *framework.TestServerConfig) {
		config.SetConsensus(framework.ConsensusDev)
		config.SetBlockLimit(blockGasLimit)
	})
	srv := srvs[0]

	client := srv.JSONRPC()

	block, err := client.Eth().GetBlockByNumber(0, true)
	if err != nil {
		t.Fatalf("failed to retreive block: %v", err)
	}

	if block.GasLimit != blockGasLimit {
		t.Fatalf("invalid block gas limit, expected [%d] but got [%d]", blockGasLimit, block.GasLimit)
	}
}

func TestGenesisDefaultBlockGasLimit(t *testing.T) {
	var blockGasLimit uint64 = helper.GenesisGasLimit
	srvs := framework.NewTestServers(t, 1, func(config *framework.TestServerConfig) {
		config.SetConsensus(framework.ConsensusDev)
	})
	srv := srvs[0]

	client := srv.JSONRPC()

	block, err := client.Eth().GetBlockByNumber(0, true)
	if err != nil {
		t.Fatalf("failed to retreive block: %v", err)
	}

	if block.GasLimit != blockGasLimit {
		t.Fatalf("invalid block gas limit, expected [%d] but got [%d]", blockGasLimit, block.GasLimit)
	}
}
