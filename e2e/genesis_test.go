package e2e

import (
	"context"
	"github.com/0xPolygon/polygon-edge/command"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/e2e/framework"
	"github.com/0xPolygon/polygon-edge/helper/tests"
	"github.com/stretchr/testify/assert"
)

// TestGenesisBlockGasLimit tests the genesis block limit setting
func TestGenesisBlockGasLimit(t *testing.T) {
	testTable := []struct {
		name                  string
		blockGasLimit         uint64
		expectedBlockGasLimit uint64
	}{
		{
			"Custom block gas limit",
			5000000000,
			5000000000,
		},
		{
			"Default block gas limit",
			0,
			command.DefaultGenesisGasLimit,
		},
	}

	for _, testCase := range testTable {
		t.Run(testCase.name, func(t *testing.T) {
			_, addr := tests.GenerateKeyAndAddr(t)

			ibftManager := framework.NewIBFTServersManager(
				t,
				1,
				IBFTDirPrefix,
				func(i int, config *framework.TestServerConfig) {
					config.Premine(addr, framework.EthToWei(10))
					config.SetBlockTime(1)

					if testCase.blockGasLimit != 0 {
						config.SetBlockLimit(testCase.blockGasLimit)
					}
				},
			)

			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()

			ibftManager.StartServers(ctx)
			srv := ibftManager.GetServer(0)

			client := srv.JSONRPC()

			block, err := client.Eth().GetBlockByNumber(0, true)
			if err != nil {
				t.Fatalf("failed to retrieve block: %v", err)
			}

			assert.Equal(t, testCase.expectedBlockGasLimit, block.GasLimit)
		})
	}
}
