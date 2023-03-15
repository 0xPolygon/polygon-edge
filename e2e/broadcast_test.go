package e2e

import (
	"context"
	"fmt"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/e2e/framework"
	"github.com/0xPolygon/polygon-edge/helper/tests"
	"github.com/0xPolygon/polygon-edge/types"
)

func TestBroadcast(t *testing.T) {
	// This test is not stable
	// Opened the ticket to check + fix it
	t.Skip()

	signer := crypto.NewSigner(chain.AllForksEnabled.At(0), 100)
	senderKey, senderAddr := tests.GenerateKeyAndAddr(t)
	_, receiverAddr := tests.GenerateKeyAndAddr(t)

	conf := func(config *framework.TestServerConfig) {
		config.SetConsensus(framework.ConsensusDummy)
		config.Premine(senderAddr, framework.EthToWei(10))
	}

	testCases := []struct {
		name     string
		numNodes int
		// Number of nodes that connects to left node
		numConnectedNodes int
		createTx          func(t *testing.T) *types.Transaction
	}{
		{
			name:              "legacy tx should not reach to last node",
			numNodes:          10,
			numConnectedNodes: 5,
			createTx: func(t *testing.T) *types.Transaction {
				t.Helper()

				tx, err := signer.SignTx(&types.Transaction{
					Nonce:    0,
					From:     senderAddr,
					To:       &receiverAddr,
					Value:    framework.EthToWei(1),
					Gas:      1000000,
					GasPrice: big.NewInt(10000),
					Input:    []byte{},
				}, senderKey)
				require.NoError(t, err, "failed to sign transaction")

				return tx
			},
		},
		{
			name:              "dynamic fee tx should not reach to last node",
			numNodes:          10,
			numConnectedNodes: 5,
			createTx: func(t *testing.T) *types.Transaction {
				t.Helper()

				tx, err := signer.SignTx(&types.Transaction{
					Type:      types.DynamicFeeTx,
					Nonce:     0,
					From:      senderAddr,
					To:        &receiverAddr,
					Value:     framework.EthToWei(1),
					Gas:       1000000,
					GasFeeCap: big.NewInt(10000),
					GasTipCap: big.NewInt(1000),
					Input:     []byte{},
				}, senderKey)
				require.NoError(t, err, "failed to sign transaction")

				return tx
			},
		},
		{
			name:              "legacy tx should reach to last node",
			numNodes:          10,
			numConnectedNodes: 10,
			createTx: func(t *testing.T) *types.Transaction {
				t.Helper()

				tx, err := signer.SignTx(&types.Transaction{
					Nonce:    0,
					From:     senderAddr,
					To:       &receiverAddr,
					Value:    framework.EthToWei(1),
					Gas:      1000000,
					GasPrice: big.NewInt(10000),
					Input:    []byte{},
				}, senderKey)
				require.NoError(t, err, "failed to sign transaction")

				return tx
			},
		},
		{
			name:              "dynamic fee tx should reach to last node",
			numNodes:          10,
			numConnectedNodes: 10,
			createTx: func(t *testing.T) *types.Transaction {
				t.Helper()

				tx, err := signer.SignTx(&types.Transaction{
					Type:      types.DynamicFeeTx,
					Nonce:     0,
					From:      senderAddr,
					To:        &receiverAddr,
					Value:     framework.EthToWei(1),
					Gas:       1000000,
					GasFeeCap: big.NewInt(10000),
					GasTipCap: big.NewInt(1000),
					Input:     []byte{},
				}, senderKey)
				require.NoError(t, err, "failed to sign transaction")

				return tx
			},
		},
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

			tx := tt.createTx(t)

			_, err := srvs[0].JSONRPC().Eth().SendRawTransaction(tx.MarshalRLP())
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
