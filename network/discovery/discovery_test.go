package discovery

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/helper/tests"
	"github.com/0xPolygon/polygon-edge/network/common"
	"github.com/0xPolygon/polygon-edge/network/proto"
	networkTesting "github.com/0xPolygon/polygon-edge/network/testing"
	"github.com/hashicorp/go-hclog"
	kb "github.com/libp2p/go-libp2p-kbucket"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
)

// newDiscoveryService creates a new discovery service instance
// with mock-able backends
func newDiscoveryService(
	networkingServerCallback func(server *networkTesting.MockNetworkingServer),
) (*DiscoveryService, error) {
	// Setup the mock networking server
	baseServer := networkTesting.NewMockNetworkingServer()

	if networkingServerCallback != nil {
		networkingServerCallback(baseServer)
	}

	// Setup the kademlia routing table
	routingTable, routingErr := kb.NewRoutingTable(
		10,
		kb.ConvertPeerID("ExampleID"),
		time.Minute,
		baseServer.GetMockPeerMetrics(),
		10*time.Second,
		nil,
	)
	if routingErr != nil {
		return nil, routingErr
	}

	return &DiscoveryService{
		baseServer:   baseServer,
		logger:       hclog.NewNullLogger(),
		routingTable: routingTable,
	}, nil
}

// getRandomPeers returns random peers, generated on the fly
func getRandomPeers(t *testing.T, count int) []*peer.AddrInfo {
	t.Helper()

	peersInfo := make([]*peer.AddrInfo, 0)

	for i := 0; i < count; i++ {
		info, err := peer.AddrInfoFromP2pAddr(
			tests.GenerateTestMultiAddr(t),
		)
		if err != nil {
			t.Fatalf("unable to generate peer info, %v", err)
		}

		peersInfo = append(peersInfo, info)
	}

	return peersInfo
}

// TestDiscoveryService_BootnodePeerDiscovery makes sure the
// discovery service's peer discovery mechanism through the bootnode works as
// expected
func TestDiscoveryService_BootnodePeerDiscovery(t *testing.T) {
	randomBootnode := &peer.AddrInfo{
		ID: "RandomBootnode",
	}
	randomPeers := getRandomPeers(t, 3)
	expectedDisconnectReason := "Thank you"

	isTemporaryDial := false
	temporaryDials := map[peer.ID]bool{
		"DummyTemp": true, // has one temporary dial for example
	}
	streamClosed := false
	disconnectReason := ""
	peerStore := make([]*peer.AddrInfo, 0)

	// Create an instance of the identity service
	discoveryService, setupErr := newDiscoveryService(
		// Set the relevant hook responses from the mock server
		func(server *networkTesting.MockNetworkingServer) {
			// Define the random bootnode hook
			server.HookGetRandomBootnode(func() *peer.AddrInfo {
				return randomBootnode
			})

			// Define the temporary dial status hook
			server.HookFetchAndSetTemporaryDial(func(id peer.ID, b bool) bool {
				isTemporaryDial = b
				temporaryDials[id] = b

				return false
			})

			// Define the temporary dial removal
			server.HookRemoveTemporaryDial(func(id peer.ID) {
				delete(temporaryDials, id)
			})

			// Define peer disconnect
			server.HookDisconnectFromPeer(func(id peer.ID, s string) {
				disconnectReason = s
			})

			// Define the bootnode conn count hook
			server.HookGetBootnodeConnCount(func() int64 {
				return 1 // > 0 to trigger a temporary connection
			})

			// Define the protocol stream closing hook
			server.HookCloseProtocolStream(func(s string, id peer.ID) error {
				if id == randomBootnode.ID {
					// Make sure the correct temporary stream is closed
					streamClosed = true
				}

				return nil
			})

			// Define the discovery client find peers hook
			server.GetMockDiscoveryClient().HookFindPeers(
				func(
					ctx context.Context,
					in *proto.FindPeersReq,
					opts ...grpc.CallOption,
				) (*proto.FindPeersResp, error) {
					// Encode the response to a string array
					peers := make([]string, len(randomPeers))

					for i, peerInfo := range randomPeers {
						// The peer info needs to be formatted as a MultiAddr
						peers[i] = common.AddrInfoToString(peerInfo)
					}

					return &proto.FindPeersResp{
						Nodes: peers,
					}, nil
				},
			)

			// Define the peer store addition
			server.HookAddToPeerStore(func(info *peer.AddrInfo) {
				peerStore = append(peerStore, info)
			})
		},
	)
	if setupErr != nil {
		t.Fatalf("Unable to setup the discovery service")
	}

	// Run the discovery service
	discoveryService.bootnodePeerDiscovery()

	// Make sure the dial was temporary
	assert.True(t, isTemporaryDial)

	// Make sure the temporary dial is removed from the server,
	// and the only one left is the initial one
	assert.Len(t, temporaryDials, 1)

	// Make sure the stream is closed to the bootnode
	assert.True(t, streamClosed)

	// Make sure the disconnect reason is matching
	assert.Equal(t, expectedDisconnectReason, disconnectReason)

	// Make sure the bootnode peers are added to the peer store
	assert.Len(t, peerStore, len(randomPeers))

	for indx, randomPeer := range randomPeers {
		assert.Equal(t, randomPeer.ID, peerStore[indx].ID)
	}
}

// TestDiscoveryService_AddToTable tests that peers are added correctly
// both to the discovery routing table, and the networking server's peer store
func TestDiscoveryService_AddToTable(t *testing.T) {
	randomPeer := getRandomPeers(t, 1)[0]
	peerStore := make(map[peer.ID]*peer.AddrInfo)

	addToPeerStoreHook := func(info *peer.AddrInfo) {
		peerStore[info.ID] = info
	}

	removeFromPeerStoreHook := func(info *peer.AddrInfo) {
		delete(peerStore, info.ID)
	}

	highLatencyHook := func(id peer.ID) time.Duration {
		// Set a high latency value to trigger an error
		return 24 * time.Hour
	}

	lowLatencyHook := func(id peer.ID) time.Duration {
		// Set a non-existing latency value
		return 0
	}

	testTable := []struct {
		name          string
		latencyHook   func(id peer.ID) time.Duration
		shouldSucceed bool
	}{
		{
			"routing table and peer store updated",
			lowLatencyHook,
			true,
		},
		{
			"peer store should not be updated due to error",
			highLatencyHook,
			false,
		},
	}

	for _, testCase := range testTable {
		t.Run(testCase.name, func(t *testing.T) {
			// Create an instance of the identity service
			discoveryService, setupErr := newDiscoveryService(
				// Set the relevant hook responses from the mock server
				func(server *networkTesting.MockNetworkingServer) {
					// Define the peer store addition
					server.HookAddToPeerStore(addToPeerStoreHook)

					// Define the peer store deletion
					server.HookRemoveFromPeerStore(removeFromPeerStoreHook)

					// Define the routing table latency hook
					server.GetMockPeerMetrics().HookLatencyEWMA(testCase.latencyHook)
				},
			)
			if setupErr != nil {
				t.Fatalf("Unable to setup the discovery service")
			}

			// Run the main method
			additionErr := discoveryService.addToTable(randomPeer)

			if testCase.shouldSucceed {
				assert.NoError(t, additionErr)
				assert.Len(t, peerStore, 1)
				assert.Equal(t, randomPeer, peerStore[randomPeer.ID])
			} else {
				assert.Error(t, additionErr)
				assert.Len(t, peerStore, 0)
			}
		})
	}
}

// TestDiscoveryService_RegularPeerDiscoveryUnconnected makes sure the peers who disconnected
// in the middle of peer discovery are not queried for their peer sets
func TestDiscoveryService_RegularPeerDiscoveryUnconnected(t *testing.T) {
	randomPeer := getRandomPeers(t, 1)[0]
	peerStore := make(map[peer.ID]*peer.AddrInfo)

	// Create an instance of the identity service
	discoveryService, setupErr := newDiscoveryService(
		// Set the relevant hook responses from the mock server
		func(server *networkTesting.MockNetworkingServer) {
			// Define the random peer hook
			server.HookGetRandomPeer(func() *peer.ID {
				return &randomPeer.ID
			})

			// Define the new discovery client creation
			server.HookNewDiscoveryClient(func(id peer.ID) (proto.DiscoveryClient, error) {
				return nil, errors.New("peer is not connected anymore")
			})

			// Define the peer store addition
			server.HookAddToPeerStore(func(info *peer.AddrInfo) {
				peerStore[info.ID] = info
			})
		},
	)
	if setupErr != nil {
		t.Fatalf("Unable to setup the discovery service")
	}

	// Run the regular peer discovery method
	discoveryService.regularPeerDiscovery()

	// Make sure that no peers were added to the peer store
	assert.Len(t, peerStore, 0)
}
