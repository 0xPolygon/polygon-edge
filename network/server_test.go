package network

import (
	"testing"
	"time"

	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/assert"
)

func TestConnLimit_Inbound(t *testing.T) {
	// we should not receive more inbound connections if we are already connected to max peers
	conf := func(c *Config) {
		c.MaxPeers = 1
		c.NoDiscover = true
	}

	srv0 := CreateServer(t, conf)
	srv1 := CreateServer(t, conf)
	srv2 := CreateServer(t, conf)

	// One slot left, it can connect 0->1
	assert.NoError(t, srv0.Join(srv1.AddrInfo(), 1*time.Second))

	// srv2 tries to connect to srv0 but srv0 is already connected
	// to max peers
	assert.Error(t, srv2.Join(srv1.AddrInfo(), 1*time.Second))

	srv0.Disconnect(srv1.host.ID(), "bye")

	// try to connect again
	assert.NoError(t, srv2.Join(srv1.AddrInfo(), 1*time.Second))
}

func TestConnLimit_Outbound(t *testing.T) {
	// we should not try to make connections if we are already connected to max peers
	conf := func(c *Config) {
		c.MaxPeers = 1
		c.NoDiscover = true
	}

	srv0 := CreateServer(t, conf)
	srv1 := CreateServer(t, conf)
	srv2 := CreateServer(t, conf)

	// One slot left, it can connect 0->1
	assert.NoError(t, srv0.Join(srv1.AddrInfo(), 1*time.Second))

	// No slots left (timeout)
	err := srv0.Join(srv2.AddrInfo(), 1*time.Second)
	assert.Error(t, err)

	// Disconnect 0 and 1 (sync)
	srv0.Disconnect(srv1.host.ID(), "bye")

	// Now srv0 is trying to connect to srv2 since there are slots left
	connected := srv0.waitForEvent(2*time.Second, connectedPeerHandler(srv2.AddrInfo().ID))
	assert.True(t, connected)
}

func TestPeersLifecycle(t *testing.T) {
	srv0 := CreateServer(t, nil)
	srv1 := CreateServer(t, nil)

	// 0 -> 1 (connect)
	assert.NoError(t, srv0.Join(srv1.AddrInfo(), 0))

	// 1 should receive the connected event as well
	assert.True(t, srv1.waitForEvent(5*time.Second, connectedPeerHandler(srv0.AddrInfo().ID)))

	// 1 -> 0 (disconnect)
	srv1.Disconnect(srv0.AddrInfo().ID, "bye")

	// both 0 and 1 should receive a disconnect event
	assert.True(t, srv0.waitForEvent(5*time.Second, disconnectedPeerHandler(srv1.AddrInfo().ID)))
}

func disconnectedPeerHandler(p peer.ID) func(evnt *PeerEvent) bool {
	return func(evnt *PeerEvent) bool {
		return evnt.Type == PeerEventDisconnected && evnt.PeerID == p
	}
}

func connectedPeerHandler(p peer.ID) func(evnt *PeerEvent) bool {
	return func(evnt *PeerEvent) bool {
		return evnt.Type == PeerEventConnected && evnt.PeerID == p
	}
}

func TestPeerEvent_EmitAndSubscribe(t *testing.T) {
	srv0 := CreateServer(t, nil)

	sub, err := srv0.Subscribe()
	assert.NoError(t, err)

	count := 10

	t.Run("serial", func(t *testing.T) {
		for i := 0; i < count; i++ {
			srv0.emitEvent(&PeerEvent{})
			sub.Get()
		}
	})

	t.Run("parallel", func(t *testing.T) {
		for i := 0; i < count; i++ {
			srv0.emitEvent(&PeerEvent{})
		}
		for i := 0; i < count; i++ {
			sub.Get()
		}
	})
}

func TestEncodingPeerAddr(t *testing.T) {
	_, pub, err := crypto.GenerateKeyPair(crypto.Secp256k1, 256)
	assert.NoError(t, err)

	id, err := peer.IDFromPublicKey(pub)
	assert.NoError(t, err)

	addr, err := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/90")
	assert.NoError(t, err)

	info := &peer.AddrInfo{
		ID:    id,
		Addrs: []multiaddr.Multiaddr{addr},
	}

	str := AddrInfoToString(info)
	info2, err := StringToAddrInfo(str)
	assert.NoError(t, err)
	assert.Equal(t, info, info2)
}

func TestJoinWhenAlreadyConnected(t *testing.T) {
	// if we try to join an already connected node, the watcher
	// should finish as well
	srv0 := CreateServer(t, nil)
	srv1 := CreateServer(t, nil)

	assert.NoError(t, srv0.Join(srv1.AddrInfo(), DefaultJoinTimeout))
	assert.NoError(t, srv0.Join(srv1.AddrInfo(), DefaultJoinTimeout))
}
