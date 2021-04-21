package e2e

import (
	"math/big"
	"testing"
	"time"

	"github.com/0xPolygon/minimal/crypto"
	"github.com/0xPolygon/minimal/e2e/framework"
	"github.com/0xPolygon/minimal/helper/hex"
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
			config.SetSeal(canSeal)
			config.Premine(senderAddr, ethToWei(10))
		})
	}
	defer func() {
		for _, srv := range srvs {
			srv.Stop()
		}
	}()

	time.Sleep(time.Second * 5)

	framework.MultiJoinSerial(t, srvs)

	time.Sleep(time.Second * 5)

	tx := &types.Transaction{
		Nonce:    0,
		From:     senderAddr,
		To:       &receiverAddr,
		Value:    ethToWei(1),
		Gas:      1000,
		GasPrice: big.NewInt(0x1000),
		Input:    []byte{},
	}
	tx, err := signer.SignTx(tx, senderKey)
	if err != nil {
		t.Fatalf("failed to sign transaction, err=%+v", err)
	}

	txHash := ""
	err = srvs[0].JSONRPC().Call("eth_sendRawTransaction", &txHash, hex.EncodeToHex(tx.MarshalRLP()))
	if err != nil {
		t.Fatalf("failed to send transaction, err=%+v", err)
	}
	t.Logf("txHash: %+v", txHash)

	time.Sleep(time.Second * 30)

	var result types.Transaction
	_ = srvs[NumOfNodes-1].JSONRPC().Call("eth_getTransactionByHash", &result, txHash)
	if err != nil {
		t.Errorf("failed to query transaction, err=%+v", err)
	}
	t.Logf("result: %+v", result)

	assert.Equal(t, txHash, result.Hash.String(), "hash doesn't equal")
	assert.Equal(t, tx.From, result.From, "from doesn't equal")
	assert.Equal(t, tx.To, result.To, "to doesn't equal")
	assert.Equal(t, tx.Value, result.Value, "value doesn't equal")
	assert.Equal(t, tx.Gas, result.Gas, "gas doesn't equal")
	assert.Equal(t, tx.GasPrice, result.GasPrice, "gas price doesn't equal")
	assert.Equal(t, tx.Input, result.Input, "input doesn't equal")
}
