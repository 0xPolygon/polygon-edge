package discovery

import networkTesting "github.com/0xPolygon/polygon-edge/network/testing"

// newDiscoveryService creates a new discovery service instance
// with mock-able backends
func newDiscoveryService(
	networkingServerCallback func(server *networkTesting.MockNetworkingServer),
) *DiscoveryService {
	baseServer := networkTesting.NewMockNetworkingServer()

	if networkingServerCallback != nil {
		networkingServerCallback(baseServer)
	}

	return &DiscoveryService{
		baseServer: baseServer,
	}
}
