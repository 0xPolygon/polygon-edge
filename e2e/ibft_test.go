package e2e

import (
	"context"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/0xPolygon/minimal/crypto"
	"github.com/0xPolygon/minimal/e2e/framework"
	"github.com/0xPolygon/minimal/types"
	"github.com/stretchr/testify/assert"
)

func TestIbft_Transfer(t *testing.T) {
	signer := &crypto.FrontierSigner{}
	senderKey, senderAddr := framework.GenerateKeyAndAddr(t)
	_, receiverAddr := framework.GenerateKeyAndAddr(t)

	dataDir, err := framework.TempDir()
	if err != nil {
		t.Fatal(err)
	}

	ibftManager := framework.NewIBFTServersManager(t, IBFTMinNodes, dataDir, IBFTDirPrefix, func(i int, config *framework.TestServerConfig) {
		config.Premine(senderAddr, framework.EthToWei(10))
		config.SetSeal(true)
	})
	t.Cleanup(func() {
		ibftManager.StopServers()
		if err := os.RemoveAll(dataDir); err != nil {
			t.Log(err)
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	ibftManager.StartServers(ctx)

	srv := ibftManager.GetServer(0)
	for i := 0; i < 3; i++ {
		txn := &types.Transaction{
			From:     senderAddr,
			To:       &receiverAddr,
			GasPrice: big.NewInt(10000),
			Gas:      1000000,
			Value:    framework.EthToWei(1),
			Nonce:    uint64(i),
		}
		txn, err = signer.SignTx(txn, senderKey)
		if err != nil {
			t.Fatal(err)
		}
		data := txn.MarshalRLP()

		hash, err := srv.JSONRPC().Eth().SendRawTransaction(data)
		assert.NoError(t, err)
		assert.NotNil(t, hash)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		receipt, err := srv.WaitForReceipt(ctx, hash)

		assert.NoError(t, err)
		assert.NotNil(t, receipt)
	}
}
