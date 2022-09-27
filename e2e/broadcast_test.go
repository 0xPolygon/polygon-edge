package e2e

import (
	"context"
	"fmt"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/e2e/framework"
	"github.com/0xPolygon/polygon-edge/helper/tests"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
)

func TestBroadcast(t *testing.T) {
	// This test is not stable
	// Opened the ticket to check + fix it
	t.Skip()

	testCases := []struct {
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
	senderKey, senderAddr := tests.GenerateKeyAndAddr(t)
	_, receiverAddr := tests.GenerateKeyAndAddr(t)

	conf := func(config *framework.TestServerConfig) {
		config.SetConsensus(framework.ConsensusDummy)
		config.Premine(senderAddr, framework.EthToWei(10))
	}

	for _, tt := range testCases {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			srvs := framework.NewTestServers(t, tt.numNodes, conf)

			framework.MultiJoinSerial(t, srvs[0:tt.numConnectedNodes])

			// Check the connections
			connectionErrors := framework.NewAtomicErrors(len(srvs))

			var wgForConnections sync.WaitGroup

			for i, srv := range srvs {
				srv := srv

				// Required number of connections
				numRequiredConnections := 0
				if i < tt.numConnectedNodes {
					if i == 0 || i == tt.numConnectedNodes-1 {
						numRequiredConnections = 1
					} else {
						numRequiredConnections = 2
					}
				}

				wgForConnections.Add(1)
				go func() {
					defer wgForConnections.Done()

					ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					defer cancel()

					_, err := framework.WaitUntilPeerConnects(ctx, srv, numRequiredConnections)
					if err != nil {
						connectionErrors.Append(err)
					}
				}()
			}

			wgForConnections.Wait()

			for _, err := range connectionErrors.Errors() {
				t.Error(err)
			}

			if len(connectionErrors.Errors()) > 0 {
				t.Fail()
			}

			// wait until gossip protocol build mesh network
			// (https://github.com/libp2p/specs/blob/master/pubsub/gossipsub/gossipsub-v1.0.md)
			time.Sleep(time.Second * 2)

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

			for i, srv := range srvs {
				srv := srv

				shouldHaveTxPool := false
				subTestName := fmt.Sprintf("node %d shouldn't have tx in txpool", i)
				if i < tt.numConnectedNodes {
					shouldHaveTxPool = true
					subTestName = fmt.Sprintf("node %d should have tx in txpool", i)
				}

				t.Run(subTestName, func(t *testing.T) {
					t.Parallel()

					ctx, cancel := context.WithTimeout(context.Background(), framework.DefaultTimeout)
					defer cancel()
					res, err := framework.WaitUntilTxPoolFilled(ctx, srv, 1)

					if shouldHaveTxPool {
						assert.NoError(t, err)
						assert.Equal(t, uint64(1), res.Length)
					} else {
						assert.ErrorIs(t, err, tests.ErrTimeout)
					}
				})
			}
		})
	}
}
