package network

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func discoveryConfig(c *Config) {
	// we limit maxPeers=1 to limit the number of connections
	// since we only want to test discovery
	c.MaxPeers = 2
}

func TestDiscovery_ConnectedPopulatesRoutingTable(t *testing.T) {
	// when two nodes connect, they populate their kademlia routing tables
	servers, createErr := createServers(2, []*CreateServerParams{nil, nil})
	if createErr != nil {
		t.Fatalf("Unable to create servers, %v", createErr)
	}

	MultiJoin(t, servers[1], servers[2])
	time.Sleep(time.Second * 2) // TODO add mesh comment

	assert.Equal(t, servers[1].discovery.routingTable.Size(), 1)
	assert.Equal(t, servers[2].discovery.routingTable.Size(), 1)
}

func TestDiscovery_ProtocolFindPeers(t *testing.T) {
	servers, createErr := createServers(2, []*CreateServerParams{nil, nil})
	if createErr != nil {
		t.Fatalf("Unable to create servers, %v", createErr)
	}

	MultiJoin(t, servers[0], servers[1])
	time.Sleep(time.Second * 2) // TODO add mesh comment

	// find peers should not include our identity
	resp, err := servers[0].discovery.findPeersCall(servers[1].AddrInfo().ID)
	assert.NoError(t, err)
	assert.Empty(t, resp)
}

func TestDiscovery_PeerAdded(t *testing.T) {
	defaultConfig := &CreateServerParams{
		ConfigCallback: discoveryConfig,
	}
	servers, createErr := createServers(3, []*CreateServerParams{defaultConfig, defaultConfig, defaultConfig})
	if createErr != nil {
		t.Fatalf("Unable to create servers, %v", createErr)
	}

	// server0 should connect to server2 by discovery
	connectedCh := asyncWaitForEvent(servers[0], 15*time.Second, connectedPeerHandler(servers[2].AddrInfo().ID))

	// serial join, srv0 -> srv1 -> srv2
	MultiJoin(t,
		servers[0], servers[1],
		servers[1], servers[2],
	)

	// wait until server0 connects to server2
	assert.True(t, <-connectedCh)
	assert.Len(t, servers[0].host.Peerstore().Peers(), 3)
	assert.Len(t, servers[1].host.Peerstore().Peers(), 3)
	assert.Len(t, servers[2].host.Peerstore().Peers(), 3)

	// TODO: We should put MaxPeers to 0 or 1 so that we do not
	// mix data and we only test how the peers are being populated
	// In theory, even if they are connected only to one peer, all of them
	// should end up with the same idea of the network.
}

func TestDiscovery_FullNetwork(t *testing.T) {
	t.Skip()

	// create a network of serially connected nodes
	// eventually, they have to find each other

	nodes := 20
	servers := []*Server{}
	for i := 0; i < nodes; i++ {
		server, createErr := CreateServer(&CreateServerParams{ConfigCallback: discoveryConfig})
		if createErr != nil {
			t.Fatalf("Unable to create server, %v", createErr)
		}
		servers = append(servers, server)
	}

	// link nodes in serial
	MultiJoinSerial(t, servers)

	// force the discover of other nodes several times
	for i := 0; i < 50; i++ {
		for _, srv := range servers {
			srv.discovery.handleDiscovery()
		}
	}

	for _, srv := range servers {
		fmt.Println("-- peerstore --")
		fmt.Println(srv.host.Peerstore().Peers().Len())
	}
}
