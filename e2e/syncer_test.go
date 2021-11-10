package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-sdk/e2e/framework"
	"github.com/stretchr/testify/assert"
	"github.com/umbracle/go-web3"
)

func TestSyncer(t *testing.T) {
	const (
		numNonValidators = 2
		desiredHeight    = 10
	)

	// Start IBFT cluster (4 Validator + 2 Non-Validator)
	ibftManager := framework.NewIBFTServersManager(t, IBFTMinNodes+numNonValidators, IBFTDirPrefix, func(i int, config *framework.TestServerConfig) {
		config.SetSeal(i < IBFTMinNodes)
	})
	ctx1, cancel1 := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(func() {
		cancel1()
	})
	ibftManager.StartServers(ctx1)

	// Wait until some blocks are mined
	ctx2, cancel2 := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(func() {
		cancel2()
	})
	_, err := framework.WaitUntilBlockMined(ctx2, ibftManager.GetServer(0), desiredHeight)
	assert.NoError(t, err)

	// Get latest block height
	latestBlockHeight, err := ibftManager.GetServer(0).GetLatestBlockHeight()
	assert.NoError(t, err)

	// Test if non-validator has latest block
	nonValidatorBlock, err := ibftManager.GetServer(IBFTMinNodes+numNonValidators-1).JSONRPC().Eth().GetBlockByNumber(web3.BlockNumber(latestBlockHeight), false)
	assert.NoError(t, err)
	assert.NotNil(t, nonValidatorBlock)
}
