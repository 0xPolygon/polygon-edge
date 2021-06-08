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
		config.Premine(senderAddr, framework.EthToWei(10))
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srvs := make([]*framework.TestServer, 0, tt.numNodes)
			for i := 0; i < tt.numNodes; i++ {
				dataDir, err := framework.TempDir()
				if err != nil {
					t.Fatal(err)
				}
				srv := framework.NewTestServer(t, dataDir, conf)
				srvs = append(srvs, srv)
			}
			t.Cleanup(func() {
				for _, srv := range srvs {
					srv.Stop()
					if err := os.RemoveAll(srv.Config.RootDir); err != nil {
						t.Log(err)
					}
				}
			})
			for _, s := range srvs {
				if err := s.GenerateGenesis(); err != nil {
					t.Fatal(err)
				}
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				if err := s.Start(ctx); err != nil {
					t.Fatal(err)
				}
			}

			framework.MultiJoinSerial(t, srvs[0:tt.numConnectedNodes])
			time.Sleep(30 * time.Second)

			tx, err := signer.SignTx(&types.Transaction{
				Nonce:    0,
				From:     senderAddr,
				To:       &receiverAddr,
				Value:    framework.EthToWei(1),
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

			time.Sleep(30 * time.Second)

			for i, srv := range srvs {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

				res, err := srv.TxnPoolOperator().Status(ctx, &emptypb.Empty{})
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
