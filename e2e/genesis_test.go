package e2e

import (
	"context"
	"github.com/0xPolygon/polygon-edge/command"
	"math/big"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/e2e/framework"
	"github.com/0xPolygon/polygon-edge/helper/tests"
	txpoolOp "github.com/0xPolygon/polygon-edge/txpool/proto"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/golang/protobuf/ptypes/any"
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

// Test if the custom block gas limit is propagated to the subsequent blocks
func TestCustomBlockGasLimitPropagation(t *testing.T) {
	var blockGasLimit uint64 = 5000000000

	senderKey, senderAddress := tests.GenerateKeyAndAddr(t)
	_, receiverAddress := tests.GenerateKeyAndAddr(t)

	srvs := framework.NewTestServers(t, 1, func(config *framework.TestServerConfig) {
		config.SetConsensus(framework.ConsensusDev)
		config.SetBlockLimit(blockGasLimit)
		config.Premine(senderAddress, framework.EthToWei(100))
		config.SetBlockGasTarget(blockGasLimit)
	})
	srv := srvs[0]

	client := srv.JSONRPC()

	block, err := client.Eth().GetBlockByNumber(0, true)
	if err != nil {
		t.Fatalf("failed to retrieve block %d: %v", 0, err)
	}

	if block.GasLimit != blockGasLimit {
		t.Fatalf("invalid block gas limit, expected [%d] but got [%d]", blockGasLimit, block.GasLimit)
	}

	signer := crypto.NewEIP155Signer(100)

	for i := 0; i < 20; i++ {
		signedTx, err := signer.SignTx(&types.Transaction{
			Nonce:    uint64(i),
			GasPrice: big.NewInt(framework.DefaultGasPrice),
			Gas:      blockGasLimit,
			To:       &receiverAddress,
			Value:    framework.EthToWei(1),
			V:        big.NewInt(1),
			From:     senderAddress,
		}, senderKey)
		if err != nil {
			t.Fatalf("failed to sign txn: %v", err)
		}

		_, err = srv.TxnPoolOperator().AddTxn(context.Background(), &txpoolOp.AddTxnReq{
			Raw: &any.Any{
				Value: signedTx.MarshalRLP(),
			},
			From: types.ZeroAddress.String(),
		})
		assert.NoError(t, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = framework.WaitUntilBlockMined(ctx, srv, 1)
	assert.NoError(t, err)

	block, err = client.Eth().GetBlockByNumber(1, true)
	assert.NoError(t, err, "failed to retrieve block %d: %v", 1, err)
	assert.NotNil(t, block)

	if block.GasLimit != blockGasLimit {
		t.Fatalf("invalid block gas limit, expected [%d] but got [%d]", blockGasLimit, block.GasLimit)
	}
}
