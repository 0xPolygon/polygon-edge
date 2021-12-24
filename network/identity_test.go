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

			// Server 0 -> Server 1
			MultiJoin(t, servers[0], servers[1])
			time.Sleep(time.Second * 2)

			subscription, subscribeErr := servers[1].Subscribe()
			if subscribeErr != nil {
				t.Fatalf("Unable to subscribe to events, %v", subscribeErr)
			}
			chainIDs := []int{
				servers[0].config.Chain.Params.ChainID,
				servers[1].config.Chain.Params.ChainID,
			}

			for {
				select {
				case <-time.After(time.Second * 5):
					subscription.Close()
					t.Fatal("Unable to receive event from peer")
				case event := <-subscription.ch:
					if chainIDs[0] == chainIDs[1] {
						// The event should've been a successful connection
						assert.Equal(t, event.Type, PeerConnected)
						assert.Equal(t, servers[0].numPeers(), int64(len(servers)-1))
						assert.Equal(t, servers[1].numPeers(), int64(len(servers)-1))
					} else {
						assert.Equal(t, event.Type, PeerDisconnected)
						assert.Equal(t, servers[0].numPeers(), int64(0))
						assert.Equal(t, servers[1].numPeers(), int64(0))
					}
				}
			}
		})
	}

}
