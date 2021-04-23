package e2e

import (
	"math/big"
	"testing"
	"time"

	"github.com/0xPolygon/minimal/crypto"
	"github.com/0xPolygon/minimal/e2e/framework"
	"github.com/0xPolygon/minimal/types"
	"github.com/stretchr/testify/assert"
)

func TestBroadcast(t *testing.T) {
	const NumOfNodes = 10

	signer := &crypto.FrontierSigner{}
	senderKey, senderAddr := framework.GenerateKeyAndAddr(t)
	_, receiverAddr := framework.GenerateKeyAndAddr(t)

	srvs := make([]*framework.TestServer, NumOfNodes)
	for i := range srvs {
		canSeal := i == NumOfNodes-1
		srvs[i] = framework.NewTestServer(t, func(config *framework.TestServerConfig) {
			config.SetConsensus(framework.ConsensusDummy)
			config.Premine(senderAddr, ethToWei(10))
			config.SetSeal(canSeal)
		})
	}
	defer func() {
		for _, srv := range srvs {
			srv.Stop()
		}
	}()

	framework.MultiJoinSerial(t, srvs)

	time.Sleep(60 * time.Second)

	tx := &types.Transaction{
		Nonce:    0,
		From:     senderAddr,
		To:       &receiverAddr,
		Value:    ethToWei(1),
		Gas:      1000000,
		GasPrice: big.NewInt(10000),
		Input:    []byte{},
	}
	tx, err := signer.SignTx(tx, senderKey)
	if err != nil {
		t.Fatalf("failed to sign transaction, err=%+v", err)
	}

	txHash, err := srvs[0].JSONRPC().Eth().SendRawTransaction(tx.MarshalRLP())
	if err != nil {
		t.Fatalf("failed to send transaction, err=%+v", err)
	}
	t.Logf("txHash: %+v", txHash)

	time.Sleep(30 * time.Second)

	res, err := srvs[NumOfNodes-1].JSONRPC().Eth().GetTransactionByHash(txHash)
	if err != nil {
		t.Errorf("failed to query transaction, err=%+v", err)
	}

	assert.NoError(t, err)
	assert.NotNil(t, res)
	assert.Equal(t, txHash.String(), res.Hash.String())
}
