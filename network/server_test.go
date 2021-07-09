package network

import (
	"fmt"
	"net"
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
	assert.NoError(t, srv0.Join(srv1.AddrInfo(), 5*time.Second))

	// srv2 tries to connect to srv0 but srv0 is already connected
	// to max peers
	assert.Error(t, srv2.Join(srv1.AddrInfo(), 5*time.Second))

	disconnectedCh := asyncWaitForEvent(srv1, 5*time.Second, disconnectedPeerHandler(srv0.AddrInfo().ID))
	srv0.Disconnect(srv1.host.ID(), "bye")
	assert.True(t, <-disconnectedCh)

	// try to connect again
	assert.NoError(t, srv2.Join(srv1.AddrInfo(), 5*time.Second))
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
	assert.NoError(t, srv0.Join(srv1.AddrInfo(), 5*time.Second))

	// No slots left (timeout)
	err := srv0.Join(srv2.AddrInfo(), 5*time.Second)
	assert.Error(t, err)

	// Disconnect 0 and 1 (sync)
	// Now srv0 is trying to connect to srv2 since there are slots left
	connectedCh := asyncWaitForEvent(srv0, 5*time.Second, connectedPeerHandler(srv2.AddrInfo().ID))
	srv0.Disconnect(srv1.host.ID(), "bye")
	assert.True(t, <-connectedCh)
}

func TestPeersLifecycle(t *testing.T) {
	conf := func(c *Config) {
		c.NoDiscover = true
	}
	srv0 := CreateServer(t, conf)
	srv1 := CreateServer(t, conf)

	// 0 -> 1 (connect)
	connectedCh := asyncWaitForEvent(srv1, 5*time.Second, connectedPeerHandler(srv0.AddrInfo().ID))
	assert.NoError(t, srv0.Join(srv1.AddrInfo(), 0))
	// 1 should receive the connected event as well
	assert.True(t, <-connectedCh)

	// 1 -> 0 (disconnect)
	disconnectedCh0 := asyncWaitForEvent(srv0, 10*time.Second, disconnectedPeerHandler(srv1.AddrInfo().ID))
	disconnectedCh1 := asyncWaitForEvent(srv1, 10*time.Second, disconnectedPeerHandler(srv0.AddrInfo().ID))
	srv1.Disconnect(srv0.AddrInfo().ID, "bye")
	// both 0 and 1 should receive a disconnect event
	assert.True(t, <-disconnectedCh0)
	assert.True(t, <-disconnectedCh1)
}

func asyncWaitForEvent(s *Server, timeout time.Duration, handler func(*PeerEvent) bool) <-chan bool {
	resCh := make(chan bool, 1)
	go func(ch chan<- bool) {
		ch <- s.waitForEvent(timeout, handler)
		close(ch)
	}(resCh)
	return resCh
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
	assert.NoError(t, srv1.Join(srv0.AddrInfo(), DefaultJoinTimeout))
}

func TestNat(t *testing.T) {
	testIP := "192.0.2.1"
	testPort := 1500 // important to be less than 2000 because of other tests and more than 1024 because of OS security
	testMultiAddrString := fmt.Sprintf("/ip4/%s/tcp/%d", testIP, testPort)

	srv := CreateServer(t, func(c *Config) {
		c.NatAddr = net.ParseIP(testIP)
		c.Addr.Port = testPort
	})
	defer srv.Close()

	t.Run("there should be multiple listening addresses", func(t *testing.T) {
		listenAddresses := srv.host.Network().ListenAddresses()

		assert.Greater(t, len(listenAddresses), 1)
	})

	t.Run("there should only be a single registered server address", func(t *testing.T) {
		addrInfo := srv.AddrInfo()
		registeredAddresses := addrInfo.Addrs

		assert.Equal(t, len(registeredAddresses), 1)
	})

	t.Run("NAT IP should not be found in listen addresses", func(t *testing.T) {
		listenAddresses := srv.host.Network().ListenAddresses()

		for _, addr := range listenAddresses {
			assert.NotEqual(t, addr.String(), testMultiAddrString)
		}
	})

	t.Run("NAT IP should be found in registered server addresses", func(t *testing.T) {
		addrInfo := srv.AddrInfo()
		registeredAddresses := addrInfo.Addrs

		found := false
		for _, addr := range registeredAddresses {
			if addr.String() == testMultiAddrString {
				found = true
				break
			}
		}

		assert.True(t, found)
	})
}
