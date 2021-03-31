package network

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDiscoveryConnectedPopulatesRoutingTable(t *testing.T) {
	// when two nodes connect, they populate their kademlia routing tables
	srv0 := createServer(t, nil)
	srv1 := createServer(t, nil)

	multiJoin(t, srv0, srv1)

	assert.Equal(t, srv0.discovery.routingTable.Size(), 1)
	assert.Equal(t, srv1.discovery.routingTable.Size(), 1)
}

func TestDiscoveryProtocolFindPeers(t *testing.T) {
	srv0 := createServer(t, nil)
	srv1 := createServer(t, nil)

	multiJoin(t, srv0, srv1)

	// find peers should not include our identity
	resp, err := srv0.discovery.findPeersCall(srv1.AddrInfo().ID)
	assert.NoError(t, err)
	assert.Empty(t, resp)
}

func TestDiscoveryPeerAdded(t *testing.T) {
	cfg := func(c *Config) {
		c.MaxPeers = 1
	}
	srv0 := createServer(t, cfg)
	srv1 := createServer(t, cfg)
	srv2 := createServer(t, cfg)

	// serial join, srv0 -> srv1 -> srv2
	multiJoin(t,
		srv0, srv1,
		srv1, srv2,
	)

	// wait for the propagation
	time.Sleep(5 * time.Second)

	assert.Len(t, srv0.host.Peerstore().Peers(), 3)
	assert.Len(t, srv1.host.Peerstore().Peers(), 3)
	assert.Len(t, srv2.host.Peerstore().Peers(), 3)

	// TODO: We should put MaxPeers to 0 or 1 so that we do not
	// mix data and we only test how the peers are being populated
	// In theory, even if they are connected only to one peer, all of them
	// should end up with the same idea of the network.
}
