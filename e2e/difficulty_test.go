package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-sdk/e2e/framework"
	"github.com/stretchr/testify/assert"
	"github.com/umbracle/go-web3"
)

func TestBlockDifficulty(t *testing.T) {
	ibftManager := framework.NewIBFTServersManager(t,
		IBFTMinNodes,
		IBFTDirPrefix,
		func(i int, config *framework.TestServerConfig) {
			config.SetSeal(true)
		})

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	ibftManager.StartServers(ctx)

	srv := ibftManager.GetServer(0)
	ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	err := srv.WaitForReady(ctx)
	if err != nil {
		t.Fatal(err)
	}

	clt := srv.JSONRPC()
	num, err := clt.Eth().BlockNumber()
	if err != nil {
		t.Fatal(err)
	}

	blk, err := clt.Eth().GetBlockByNumber(web3.BlockNumber(num), false)
	if err != nil {
		t.Fatal(err)
	}

	// verify that block difficulty is 1
	assert.Equal(t, blk.Difficulty.Uint64(), uint64(1))
}
