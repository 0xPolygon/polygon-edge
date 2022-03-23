package identity

import (
	"context"
	"github.com/0xPolygon/polygon-edge/network/proto"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"testing"
)

// newIdentityService creates a new identity service instance
// with mock-able backends
func newIdentityService(
	networkingServerCallback func(server *mockNetworkingServer),
) *IdentityService {
	baseServer := newMockNetworkingServer()

	if networkingServerCallback != nil {
		networkingServerCallback(baseServer)
	}

	return &IdentityService{
		baseServer: baseServer,
	}
}

// TestTemporaryDial tests temporary peer connections,
// by making sure temporary dials aren't saved as persistent peers
func TestTemporaryDial(t *testing.T) {
	peersArray := make([]peer.ID, 0)

	// Create an instance of the identity service
	identityService := newIdentityService(
		// Set the relevant hook responses from the mock server
		func(server *mockNetworkingServer) {
			// Define the temporary dial hook
			server.isTemporaryDialFn = func(peerID peer.ID) bool {
				return true
			}

			// Define the add peer hook
			server.addPeerFn = func(
				id peer.ID,
				direction network.Direction,
			) {
				peersArray = append(peersArray, id)
			}

			// Define the mock IdentityClient response
			server.mockClient.helloFn = func(
				ctx context.Context,
				in *proto.Status,
				opts ...grpc.CallOption,
			) (*proto.Status, error) {
				return &proto.Status{
					Chain:         0,
					TemporaryDial: true, // make sure the dial is temporary
				}, nil
			}
		},
	)

	// Check that there was no error during handshaking
	assert.NoError(
		t,
		identityService.handleConnected("TestPeer", network.DirInbound),
	)

	// Make sure no peers have been  added to the base networking server
	assert.Len(t, peersArray, 0)
}

// TestHandshake_Errors tests peer connections errors
func TestHandshake_Errors(t *testing.T) {
	peersArray := make([]peer.ID, 0)
	requesterChainID := int64(1)
	responderChainID := requesterChainID + 1 // different chain ID

	// Create an instance of the identity service
	identityService := newIdentityService(
		// Set the relevant hook responses from the mock server
		func(server *mockNetworkingServer) {
			// Define the add peer hook
			server.addPeerFn = func(
				id peer.ID,
				direction network.Direction,
			) {
				peersArray = append(peersArray, id)
			}

			// Define the mock IdentityClient response
			server.mockClient.helloFn = func(
				ctx context.Context,
				in *proto.Status,
				opts ...grpc.CallOption,
			) (*proto.Status, error) {
				return &proto.Status{
					Chain:         responderChainID,
					TemporaryDial: false,
				}, nil
			}
		},
	)

	// Set the requester chain ID
	identityService.chainID = requesterChainID

	// Check that there was a chain ID mismatch during handshaking
	connectErr := identityService.handleConnected("TestPeer", network.DirInbound)
	if connectErr == nil {
		t.Fatalf("no connection error occurred")
	}

	assert.ErrorIs(t, connectErr, ErrInvalidChainID)

	// Make sure no peers have been  added to the base networking server
	assert.Len(t, peersArray, 0)
}
