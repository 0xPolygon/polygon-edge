package network

import (
	"github.com/0xPolygon/polygon-sdk/chain"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestIdentityHandshake(t *testing.T) {
	defaultChainID := 100

	testTable := []struct {
		name    string
		chainId int
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
			servers, createErr := createServers(2, []*CreateServerParams{
				{
					ConfigCallback: func(c *Config) {
						c.Chain.Params = &chain.Params{
							ChainID: defaultChainID,
						}
					},
				},
				{
					ConfigCallback: func(c *Config) {
						c.Chain.Params = &chain.Params{
							ChainID: testCase.chainId,
						}
					},
				},
			})
			if createErr != nil {
				t.Fatalf("Unable to create servers, %v", createErr)
			}

			t.Cleanup(func() {
				for _, server := range servers {
					assert.NoError(t, server.Close())
				}
			})

			chainIDs := []int{
				servers[0].config.Chain.Params.ChainID,
				servers[1].config.Chain.Params.ChainID,
			}

			shouldSucceed := chainIDs[0] == chainIDs[1]

			// Server 0 -> Server 1
			joinErr := servers[0].Join(servers[1].AddrInfo(), 5*time.Second)
			time.Sleep(time.Second * 2) // TODO add mesh comment

			if shouldSucceed && joinErr != nil {
				t.Fatalf("Unable to join peer, %v", joinErr)
			}

			if shouldSucceed {
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
