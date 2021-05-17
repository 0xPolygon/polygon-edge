package e2e

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/0xPolygon/minimal/crypto"
	"github.com/0xPolygon/minimal/e2e/framework"
	"github.com/0xPolygon/minimal/types"
	"google.golang.org/protobuf/types/known/emptypb"
)

func TestBroadcast(t *testing.T) {
	tests := []struct {
		name     string
		numNodes int
		// Number of nodes that connects to left node
		numConnectedNodes int
	}{
		{
			name:              "tx should not reach to last node",
			numNodes:          10,
			numConnectedNodes: 5,
		},
		{
			name:              "tx should reach to last node",
			numNodes:          10,
			numConnectedNodes: 10,
		},
	}

	signer := &crypto.FrontierSigner{}
	senderKey, senderAddr := framework.GenerateKeyAndAddr(t)
	_, receiverAddr := framework.GenerateKeyAndAddr(t)

	conf := func(config *framework.TestServerConfig) {
		config.SetConsensus(framework.ConsensusDummy)
		config.SetSeal(true)
		config.Premine(senderAddr, ethToWei(10))
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srvs := make([]*framework.TestServer, tt.numNodes)
			for i := range srvs {
				srvs[i] = framework.NewTestServer(t, conf)
			}
			defer func() {
				for _, srv := range srvs {
					srv.Stop()
				}
			}()

			framework.MultiJoinSerial(t, srvs[0:tt.numConnectedNodes])
			time.Sleep(20 * time.Second)

			tx, err := signer.SignTx(&types.Transaction{
				Nonce:    0,
				From:     senderAddr,
				To:       &receiverAddr,
				Value:    ethToWei(1),
				Gas:      1000000,
				GasPrice: big.NewInt(10000),
				Input:    []byte{},
			}, senderKey)
			if err != nil {
				t.Fatalf("failed to sign transaction, err=%+v", err)
			}

			_, err = srvs[0].JSONRPC().Eth().SendRawTransaction(tx.MarshalRLP())
			if err != nil {
				t.Fatalf("failed to send transaction, err=%+v", err)
			}

			time.Sleep(20 * time.Second)

			for i, srv := range srvs {
				res, err := srv.TxnPoolOperator().Status(context.Background(), &emptypb.Empty{})
				if err != nil {
					t.Error(err)
					continue
				}

				numStoredTx := 0
				if i < tt.numConnectedNodes {
					numStoredTx = 1
				}
				if res.Length != uint64(numStoredTx) {
					t.Errorf("node %d expected to store %d transactions, but got %d", i, numStoredTx, res.Length)
				}
			}
		})
	}
}
