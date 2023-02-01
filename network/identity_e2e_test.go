package network

import (
	"context"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/stretchr/testify/assert"
)

func TestIdentityHandshake(t *testing.T) {
	defaultChainID := int64(100)

	testTable := []struct {
		name    string
		chainID int64
	}{
		{
			"Successful handshake (same chain ID)",
			defaultChainID,
		},
		{
			"Unsuccessful handshake (different chain ID)",
			defaultChainID + defaultChainID,
		},
	}

	for _, testCase := range testTable {
		t.Run(testCase.name, func(t *testing.T) {
			params := map[int]*CreateServerParams{
				0: {
					ConfigCallback: func(c *Config) {
						c.Chain.Params = &chain.Params{
							ChainID: defaultChainID,
						}
					},
				},
				1: {
					ConfigCallback: func(c *Config) {
						c.Chain.Params = &chain.Params{
							ChainID: testCase.chainID,
						}
					},
				},
			}
			servers, createErr := createServers(2, params)
			if createErr != nil {
				t.Fatalf("Unable to create servers, %v", createErr)
			}

			t.Cleanup(func() {
				closeTestServers(t, servers)
			})

			chainIDs := []int64{
				servers[0].config.Chain.Params.ChainID,
				servers[1].config.Chain.Params.ChainID,
			}

			shouldSucceed := chainIDs[0] == chainIDs[1]

			// Server 0 -> Server 1
			joinTimeout := DefaultJoinTimeout
			connectTimeout := DefaultBufferTimeout
			if !shouldSucceed {
				connectTimeout = time.Second * 5
				joinTimeout = time.Second * 5
			}

			joinErr := JoinAndWait(servers[0], servers[1], connectTimeout, joinTimeout)
			if shouldSucceed && joinErr != nil {
				t.Fatalf("Unable to join peer, %v", joinErr)
			}

			if shouldSucceed {
				// Wait until Server 1 also has a connection to Server 0 before asserting
				connectCtx, connectFn := context.WithTimeout(context.Background(), connectTimeout)
				defer connectFn()

				if _, connectErr := WaitUntilPeerConnectsTo(
					connectCtx,
					servers[1],
					servers[0].AddrInfo().ID,
				); connectErr != nil {
					t.Fatalf("Unable to wait for connection between Server 1 to Server 0, %v", connectErr)
				}

				// Peer has been successfully added
				assert.Equal(t, servers[0].numPeers(), int64(len(servers)-1))
				assert.Equal(t, servers[1].numPeers(), int64(len(servers)-1))
			} else {
				// No peer has been added
				assert.Equal(t, servers[0].numPeers(), int64(0))
				assert.Equal(t, servers[1].numPeers(), int64(0))
			}
		})
	}
}
