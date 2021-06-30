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
	srv0 := CreateServer(t, nil)
	srv1 := CreateServer(t, nil)

	MultiJoin(t, srv0, srv1)
	time.Sleep(1 * time.Second)

	assert.Equal(t, srv0.discovery.routingTable.Size(), 1)
	assert.Equal(t, srv1.discovery.routingTable.Size(), 1)
}

func TestDiscovery_ProtocolFindPeers(t *testing.T) {
	srv0 := CreateServer(t, nil)
	srv1 := CreateServer(t, nil)

	MultiJoin(t, srv0, srv1)
	time.Sleep(1 * time.Second)

	// find peers should not include our identity
	resp, err := srv0.discovery.findPeersCall(srv1.AddrInfo().ID)
	assert.NoError(t, err)
	assert.Empty(t, resp)
}

func TestDiscovery_PeerAdded(t *testing.T) {
	srv0 := CreateServer(t, discoveryConfig)
	srv1 := CreateServer(t, discoveryConfig)
	srv2 := CreateServer(t, discoveryConfig)

	// server0 should connect to server2 by discovery
	connectedCh := asyncWaitForEvent(srv0, 15*time.Second, connectedPeerHandler(srv2.AddrInfo().ID))

	// serial join, srv0 -> srv1 -> srv2
	MultiJoin(t,
		srv0, srv1,
		srv1, srv2,
	)

	// wait until server0 connects to server2
	assert.True(t, <-connectedCh)
	assert.Len(t, srv0.host.Peerstore().Peers(), 3)
	assert.Len(t, srv1.host.Peerstore().Peers(), 3)
	assert.Len(t, srv2.host.Peerstore().Peers(), 3)

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
		srv := CreateServer(t, discoveryConfig)
		servers = append(servers, srv)
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
