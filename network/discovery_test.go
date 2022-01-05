package network

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func discoveryConfig(c *Config) {
	// we limit maxPeers=1 to limit the number of connections
	// since we only want to test discovery
	c.MaxInboundPeers = 2
	c.MaxOutboundPeers = 2
}

func TestDiscovery_ConnectedPopulatesRoutingTable(t *testing.T) {
	// when two nodes connect, they populate their kademlia routing tables
	servers, createErr := createServers(2, nil)
	if createErr != nil {
		t.Fatalf("Unable to create servers, %v", createErr)
	}
	t.Cleanup(func() {
		closeTestServers(t, servers)
	})

	joinErr := JoinAndWait(servers[0], servers[1], DefaultBufferTimeout, DefaultJoinTimeout)
	if joinErr != nil {
		t.Fatalf("Unable to join peers, %v", joinErr)
	}

	assert.Equal(t, servers[0].discovery.routingTable.Size(), 1)
	assert.Equal(t, servers[1].discovery.routingTable.Size(), 1)
}

func TestDiscovery_ProtocolFindPeers(t *testing.T) {
	servers, createErr := createServers(2, nil)
	if createErr != nil {
		t.Fatalf("Unable to create servers, %v", createErr)
	}
	t.Cleanup(func() {
		closeTestServers(t, servers)
	})

	joinErr := JoinAndWait(servers[0], servers[1], DefaultBufferTimeout, DefaultJoinTimeout)
	if joinErr != nil {
		t.Fatalf("Unable to join peers, %v", joinErr)
	}

	// find peers should not include our identity
	resp, err := servers[0].discovery.findPeersCall(servers[1].AddrInfo().ID)
	assert.NoError(t, err)
	assert.Empty(t, resp)
}

func TestDiscovery_PeerAdded(t *testing.T) {
	defaultConfig := &CreateServerParams{
		ConfigCallback: discoveryConfig,
	}
	paramsMap := map[int]*CreateServerParams{
		0: defaultConfig,
		1: defaultConfig,
		2: defaultConfig,
	}

	servers, createErr := createServers(3, paramsMap)
	if createErr != nil {
		t.Fatalf("Unable to create servers, %v", createErr)
	}
	t.Cleanup(func() {
		closeTestServers(t, servers)
	})

	// Server 0 -> Server 1
	if joinErr := JoinAndWait(servers[0], servers[1], DefaultBufferTimeout, DefaultJoinTimeout); joinErr != nil {
		t.Fatalf("Unable to join peers, %v", joinErr)
	}

	// Server 1 -> Server 2
	if joinErr := JoinAndWait(servers[1], servers[2], DefaultBufferTimeout, DefaultJoinTimeout); joinErr != nil {
		t.Fatalf("Unable to join peers, %v", joinErr)
	}

	// Wait until Server 0 connects to Server 2 by discovery
	discoveryTimeout := time.Second * 25
	connectCtx, connectFn := context.WithTimeout(context.Background(), discoveryTimeout)
	defer connectFn()
	if _, connectErr := WaitUntilPeerConnectsTo(
		connectCtx,
		servers[0],
		servers[2].AddrInfo().ID,
	); connectErr != nil {
		t.Fatalf("Unable to connect to peer, %v", connectErr)
	}

	// Check that all peers are connected to each other
	for _, server := range servers {
		assert.Len(t, server.host.Peerstore().Peers(), 3)
	}
}
