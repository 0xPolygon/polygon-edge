package network

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-sdk/helper/tests"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/assert"
)

func TestConnLimit_Inbound(t *testing.T) {
	// we should not receive more inbound connections if we are already connected to max peers
	defaultConfig := &CreateServerParams{
		ConfigCallback: func(c *Config) {
			c.MaxPeers = 1
			c.NoDiscover = true
		},
	}

	servers, createErr := createServers(3, []*CreateServerParams{defaultConfig, defaultConfig, defaultConfig})
	if createErr != nil {
		t.Fatalf("Unable to create servers, %v", createErr)
	}

	// One slot left, it can connect 0->1
	assert.NoError(t, servers[0].Join(servers[1].AddrInfo(), 5*time.Second))

	// servers[2] tries to connect to servers[0] but servers[0] is already connected
	// to max peers
	assert.Error(t, servers[2].Join(servers[1].AddrInfo(), 5*time.Second))

	disconnectedCh := asyncWaitForEvent(servers[1], 5*time.Second, disconnectedPeerHandler(servers[0].AddrInfo().ID))
	servers[0].Disconnect(servers[1].host.ID(), "bye")
	assert.True(t, <-disconnectedCh)

	// try to connect again
	assert.NoError(t, servers[2].Join(servers[1].AddrInfo(), 5*time.Second))
}

func TestConnLimit_Outbound(t *testing.T) {
	defaultConfig := &CreateServerParams{
		ConfigCallback: func(c *Config) {
			c.MaxPeers = 1
			c.NoDiscover = true
		},
	}

	servers, createErr := createServers(3, []*CreateServerParams{defaultConfig, defaultConfig, defaultConfig})
	if createErr != nil {
		t.Fatalf("Unable to create servers, %v", createErr)
	}

	// One slot left, it can connect 0->1
	assert.NoError(t, servers[0].Join(servers[1].AddrInfo(), 5*time.Second))

	// No slots left (timeout)
	err := servers[0].Join(servers[2].AddrInfo(), 3*time.Second)
	assert.Error(t, err)

	// Disconnect 0 and 1 (sync)
	// Now server 0 is trying to connect to server 2 since there are slots left
	connectedCh := asyncWaitForEvent(servers[0], 5*time.Second, connectedPeerHandler(servers[2].AddrInfo().ID))
	servers[0].Disconnect(servers[1].host.ID(), "bye")
	assert.True(t, <-connectedCh)
}

func TestPeersLifecycle(t *testing.T) {
	defaultConfig := &CreateServerParams{
		ConfigCallback: func(c *Config) {
			c.NoDiscover = true
		},
	}

	servers, createErr := createServers(3, []*CreateServerParams{defaultConfig, defaultConfig, defaultConfig})
	if createErr != nil {
		t.Fatalf("Unable to create servers, %v", createErr)
	}

	// 0 -> 1 (connect)
	connectedCh := asyncWaitForEvent(servers[1], 5*time.Second, connectedPeerHandler(servers[0].AddrInfo().ID))
	assert.NoError(t, servers[0].Join(servers[1].AddrInfo(), 0))
	// 1 should receive the connected event as well
	assert.True(t, <-connectedCh)

	// 1 -> 0 (disconnect)
	disconnectedCh0 := asyncWaitForEvent(servers[0], 10*time.Second, disconnectedPeerHandler(servers[1].AddrInfo().ID))
	disconnectedCh1 := asyncWaitForEvent(servers[1], 10*time.Second, disconnectedPeerHandler(servers[0].AddrInfo().ID))
	servers[1].Disconnect(servers[0].AddrInfo().ID, "bye")
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
		return evnt.Type == PeerDisconnected && evnt.PeerID == p
	}
}

func connectedPeerHandler(p peer.ID) func(evnt *PeerEvent) bool {
	return func(evnt *PeerEvent) bool {
		return evnt.Type == PeerConnected && evnt.PeerID == p
	}
}

func TestPeerEvent_EmitAndSubscribe(t *testing.T) {
	server, createErr := CreateServer(&CreateServerParams{ConfigCallback: func(c *Config) {
		c.NoDiscover = true
	}})
	if createErr != nil {
		t.Fatalf("Unable to create server, %v", createErr)
	}

	sub, err := server.Subscribe()
	assert.NoError(t, err)

	count := 10
	events := []PeerEventType{
		PeerConnected,
		PeerFailedToConnect,
		PeerDisconnected,
		PeerAlreadyConnected,
		PeerDialCompleted,
		PeerAddedToDialQueue,
	}

	getIDAndEventType := func(i int) (peer.ID, PeerEventType) {
		id := peer.ID(strconv.Itoa(i))
		event := events[i%len(events)]
		return id, event
	}

	t.Run("serial", func(t *testing.T) {
		for i := 0; i < count; i++ {
			id, event := getIDAndEventType(i)
			server.emitEvent(id, event)

			received := sub.Get()
			assert.Equal(t, &PeerEvent{id, event}, received)
		}
	})

	t.Run("parallel", func(t *testing.T) {
		for i := 0; i < count; i++ {
			id, event := getIDAndEventType(i)
			server.emitEvent(id, event)
		}
		for i := 0; i < count; i++ {
			received := sub.Get()
			id, event := getIDAndEventType(i)
			assert.Equal(t, &PeerEvent{id, event}, received)
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

// constructMultiAddrs is a helper function for converting raw IPs to mutliaddrs
func constructMultiAddrs(addresses []string) ([]multiaddr.Multiaddr, error) {
	returnAddrs := make([]multiaddr.Multiaddr, 0)

	for _, addr := range addresses {
		multiAddr, err := multiaddr.NewMultiaddr(addr)
		if err != nil {
			return nil, err
		}

		returnAddrs = append(returnAddrs, multiAddr)
	}

	return returnAddrs, nil
}

func TestAddrInfoToString(t *testing.T) {
	defaultPeerID := peer.ID("123")
	defaultPort := 5000

	// formatMultiaddr is a helper function for formatting an IP / port combination
	formatMultiaddr := func(ipVersion string, address string, port int) string {
		return fmt.Sprintf("/%s/%s/tcp/%d", ipVersion, address, port)
	}

	// formatExpectedOutput is a helper function for generating the expected output for AddrInfoToString
	formatExpectedOutput := func(input string, peerID string) string {
		return fmt.Sprintf("%s/p2p/%s", input, peerID)
	}

	testTable := []struct {
		name      string
		addresses []string
		expected  string
	}{
		{
			// A host address is explicitly specified
			"Valid dial address found",
			[]string{formatMultiaddr("ip4", "192.168.1.1", defaultPort)},
			formatExpectedOutput(
				formatMultiaddr("ip4", "192.168.1.1", defaultPort),
				defaultPeerID.String(),
			),
		},
		{
			// Multiple addresses that the host can listen on (0.0.0.0 special case)
			"Ignore IPv4 loopback address",
			[]string{
				formatMultiaddr("ip4", "127.0.0.1", defaultPort),
				formatMultiaddr("ip4", "192.168.1.1", defaultPort),
			},
			formatExpectedOutput(
				formatMultiaddr("ip4", "192.168.1.1", defaultPort),
				defaultPeerID.String(),
			),
		},
		{
			// Multiple addresses that the host can listen on (0.0.0.0 special case)
			"Ignore IPv6 loopback addresses",
			[]string{
				formatMultiaddr("ip6", "0:0:0:0:0:0:0:1", defaultPort),
				formatMultiaddr("ip6", "::1", defaultPort),
				formatMultiaddr("ip6", "2001:0db8:85a3:0000:0000:8a2e:0370:7334", defaultPort),
			},
			formatExpectedOutput(
				formatMultiaddr("ip6", "2001:db8:85a3::8a2e:370:7334", defaultPort),
				defaultPeerID.String(),
			),
		},
	}

	for _, testCase := range testTable {
		t.Run(testCase.name, func(t *testing.T) {
			// Construct the multiaddrs
			multiAddrs, constructErr := constructMultiAddrs(testCase.addresses)
			if constructErr != nil {
				t.Fatalf("Unable to construct multiaddrs, %v", constructErr)
			}

			dialAddress := AddrInfoToString(&peer.AddrInfo{
				ID:    defaultPeerID,
				Addrs: multiAddrs,
			})

			assert.Equal(t, testCase.expected, dialAddress)
		})
	}
}

func TestJoinWhenAlreadyConnected(t *testing.T) {
	// if we try to join an already connected node, the watcher
	// should finish as well
	servers, createErr := createServers(2, []*CreateServerParams{nil, nil})
	if createErr != nil {
		t.Fatalf("Unable to create servers, %v", createErr)
	}

	assert.NoError(t, servers[0].Join(servers[1].AddrInfo(), DefaultJoinTimeout))
	assert.NoError(t, servers[1].Join(servers[0].AddrInfo(), DefaultJoinTimeout))
}

func TestNat(t *testing.T) {
	testIP := "192.0.2.1"
	testPort := 1500 // important to be less than 2000 because of other tests and more than 1024 because of OS security
	testMultiAddrString := fmt.Sprintf("/ip4/%s/tcp/%d", testIP, testPort)

	server, createErr := CreateServer(&CreateServerParams{ConfigCallback: func(c *Config) {
		c.NatAddr = net.ParseIP(testIP)
		c.Addr.Port = testPort
	}})
	if createErr != nil {
		t.Fatalf("Unable to create server, %v", createErr)
	}

	t.Cleanup(func() {
		assert.NoError(t, server.Close())
	})

	// There should be multiple listening addresses
	listenAddresses := server.host.Network().ListenAddresses()
	assert.Greater(t, len(listenAddresses), 1)

	// NAT IP should not be found in listen addresses
	for _, addr := range listenAddresses {
		assert.NotEqual(t, addr.String(), testMultiAddrString)
	}

	// There should only be a singly registered server address
	addrInfo := server.AddrInfo()
	registeredAddresses := addrInfo.Addrs

	assert.Equal(t, len(registeredAddresses), 1)

	// NAT IP should be found in registered server addresses
	found := false
	for _, addr := range registeredAddresses {
		if addr.String() == testMultiAddrString {
			found = true
			break
		}
	}

	assert.True(t, found)
}

func WaitUntilPeerConnectsTo(ctx context.Context, srv *Server, ids ...peer.ID) (bool, error) {
	res, err := tests.RetryUntilTimeout(ctx, func() (interface{}, bool) {
		for _, v := range ids {
			if _, ok := srv.peers[v]; ok {
				return true, false
			}
		}
		return nil, true

	})
	if err != nil {
		return false, err
	}
	return res.(bool), nil
}

// TestPeerReconnection checks whether the node is able to reconnect with bootnodes on losing all active connections
func TestPeerReconnection(t *testing.T) {
	bootnodeConfig := &CreateServerParams{
		ConfigCallback: func(c *Config) {
			c.MaxPeers = 3
			c.NoDiscover = false
		},
	}
	//Create bootnodes
	bootnodes, createErr := createServers(2, []*CreateServerParams{bootnodeConfig, bootnodeConfig})
	if createErr != nil {
		t.Fatalf("Unable to create servers, %v", createErr)
	}

	defaultConfig := &CreateServerParams{
		ConfigCallback: func(c *Config) {
			c.MaxPeers = 3
			c.NoDiscover = false
			c.Chain.Bootnodes = []string{
				AddrInfoToString(bootnodes[0].AddrInfo()),
				AddrInfoToString(bootnodes[1].AddrInfo()),
			}
		},
	}

	servers, createErr := createServers(2, []*CreateServerParams{defaultConfig, defaultConfig})
	if createErr != nil {
		t.Fatalf("Unable to create servers, %v", createErr)
	}

	//connect with the first boot node
	connectedCh1 := asyncWaitForEvent(servers[0], 10*time.Second, connectedPeerHandler(bootnodes[0].AddrInfo().ID))
	assert.True(t, <-connectedCh1)

	//connect with the second boot node
	connectedCh2 := asyncWaitForEvent(servers[0], 10*time.Second, connectedPeerHandler(bootnodes[1].AddrInfo().ID))
	assert.True(t, <-connectedCh2)

	assert.NoError(t, servers[0].Join(servers[1].AddrInfo(), 5*time.Second))

	//disconnect from the first boot node
	disconnectedCh1 := asyncWaitForEvent(servers[0], 15*time.Second, disconnectedPeerHandler(bootnodes[0].AddrInfo().ID))
	servers[0].Disconnect(bootnodes[0].AddrInfo().ID, "Bye")

	assert.True(t, <-disconnectedCh1, "Failed to receive peer disconnected event")

	//disconnect from the second boot node
	disconnectedCh2 := asyncWaitForEvent(servers[0], 15*time.Second, disconnectedPeerHandler(bootnodes[1].AddrInfo().ID))
	servers[0].Disconnect(bootnodes[1].AddrInfo().ID, "Bye")

	assert.True(t, <-disconnectedCh2, "Failed to receive peer disconnected event")

	//disconnect from the third  node
	disconnectedCh3 := asyncWaitForEvent(servers[0], 15*time.Second, disconnectedPeerHandler(servers[1].AddrInfo().ID))
	assert.NoError(t, servers[1].Close())
	assert.True(t, <-disconnectedCh3)

	waitCtx, cancelWait := context.WithTimeout(context.Background(), time.Second*100)
	defer cancelWait()
	reconnected, err := WaitUntilPeerConnectsTo(waitCtx, servers[0], bootnodes[0].host.ID(), bootnodes[1].host.ID())
	assert.NoError(t, err)
	assert.True(t, reconnected)

}
func TestReconnectionWithNewIP(t *testing.T) {
	natIP := "127.0.0.1"

	_, dir0 := GenerateTestLibp2pKey(t)
	_, dir1 := GenerateTestLibp2pKey(t)

	defaultConfig := func(c *Config) {
		c.NoDiscover = true
	}

	servers, createErr := createServers(3,
		[]*CreateServerParams{
			{
				ConfigCallback: func(c *Config) {
					defaultConfig(c)
					c.DataDir = dir0
				},
			},
			{
				ConfigCallback: func(c *Config) {
					defaultConfig(c)
					c.DataDir = dir1
				},
			},
			{
				ConfigCallback: func(c *Config) {
					defaultConfig(c)
					c.DataDir = dir1
					c.NatAddr = net.ParseIP(natIP)
				},
			},
		},
	)
	if createErr != nil {
		t.Fatalf("Unable to create servers, %v", createErr)
	}

	// same ID to but diffrent IP from servers[1]

	t.Cleanup(func() {
		_ = servers[0].Close()
		_ = servers[1].Close()
		_ = servers[2].Close()
	})

	// servers[0] connects to servers[1]
	connectedCh := asyncWaitForEvent(servers[1], 10*time.Second, connectedPeerHandler(servers[0].AddrInfo().ID))
	assert.NoError(t, servers[0].Join(servers[1].AddrInfo(), DefaultJoinTimeout))
	assert.True(t, <-connectedCh)
	assert.Len(t, servers[0].peers, 1)
	assert.Len(t, servers[1].peers, 1)

	// servers[1] terminates
	disconnectedCh := asyncWaitForEvent(servers[0], 10*time.Second, disconnectedPeerHandler(servers[1].AddrInfo().ID))
	servers[1].host.Close()
	assert.True(t, <-disconnectedCh)

	// servers[0] connects to servers[2]
	connectedCh = asyncWaitForEvent(servers[2], 10*time.Second, connectedPeerHandler(servers[0].AddrInfo().ID))
	assert.NoError(t, servers[0].Join(servers[2].AddrInfo(), DefaultJoinTimeout))
	assert.True(t, <-connectedCh)
	assert.Len(t, servers[0].peers, 1)
	assert.Len(t, servers[2].peers, 1)
}

func TestSelfConnection_WithBootNodes(t *testing.T) {

	//Create a temporary directory for storing the key file
	key, directoryName := GenerateTestLibp2pKey(t)
	peerId, err := peer.IDFromPrivateKey(key)
	assert.NoError(t, err)
	testMultiAddr := GenerateTestMultiAddr(t).String()
	peerAddressInfo, err := StringToAddrInfo(testMultiAddr)
	assert.NoError(t, err)

	testTable := []struct {
		name         string
		bootNodes    []string
		expectedList []*peer.AddrInfo
	}{

		{
			name:         "Should return an non empty bootnodes list",
			bootNodes:    []string{"/ip4/127.0.0.1/tcp/10001/p2p/" + peerId.Pretty(), testMultiAddr},
			expectedList: []*peer.AddrInfo{peerAddressInfo},
		},
	}

	for _, tt := range testTable {
		t.Run(tt.name, func(t *testing.T) {
			server, createErr := CreateServer(&CreateServerParams{ConfigCallback: func(c *Config) {
				c.NoDiscover = false
				c.DataDir = directoryName
				c.Chain.Bootnodes = tt.bootNodes
			}})
			if createErr != nil {
				t.Fatalf("Unable to create server, %v", createErr)
			}

			assert.Equal(t, server.discovery.bootnodes, tt.expectedList)
		})
	}
}

func TestRunDial(t *testing.T) {
	// setupServers returns server and list of peer's server
	setupServers := func(t *testing.T, maxPeers []uint64) []*Server {
		servers := make([]*Server, len(maxPeers))
		for idx := range servers {
			server, createErr := CreateServer(&CreateServerParams{ConfigCallback: func(c *Config) {
				c.MaxPeers = maxPeers[idx]
				c.NoDiscover = true
			}})
			if createErr != nil {
				t.Fatalf("Unable to create servers, %v", createErr)
			}

			servers[idx] = server
		}

		return servers
	}

	connectToPeer := func(srv *Server, peer *peer.AddrInfo, timeout time.Duration) bool {
		connectedCh := asyncWaitForEvent(srv, timeout, connectedPeerHandler(peer.ID))
		_ = srv.Join(peer, 0)
		return <-connectedCh
	}

	closeServers := func(servers ...*Server) {
		for _, s := range servers {
			_ = s.Close()
		}
	}

	t.Run("should connect to all peers", func(t *testing.T) {
		maxPeers := []uint64{2, 1, 1}
		servers := setupServers(t, maxPeers)
		srv, peers := servers[0], servers[1:]

		for idx, p := range peers {
			addr := p.AddrInfo()
			connected := connectToPeer(srv, addr, 5*time.Second)
			assert.Truef(t, connected, "should connect to peer %d[%s], but didn't\n", idx, addr.ID)
		}
		closeServers(servers...)
	})

	t.Run("should fail to connect to some peers due to reaching limit", func(t *testing.T) {
		maxPeers := []uint64{2, 1, 1, 1}
		servers := setupServers(t, maxPeers)
		srv, peers := servers[0], servers[1:]

		for idx, p := range peers {
			addr := p.AddrInfo()
			connected := connectToPeer(srv, addr, 5*time.Second)
			if uint64(idx) < maxPeers[0] {
				assert.Truef(t, connected, "should connect to peer %d[%s], but didn't\n", idx, addr.ID)
			} else {
				assert.Falsef(t, connected, "should fail to connect to peer %d[%s], but connected\n", idx, addr.ID)
			}
		}
		closeServers(servers...)
	})

	t.Run("should try to connect after adding a peer to queue", func(t *testing.T) {
		// peer1 can't connect to any peer
		maxPeers := []uint64{1, 0, 1}
		servers := setupServers(t, maxPeers)
		srv, peers := servers[0], servers[1:]

		connected := connectToPeer(srv, peers[0].AddrInfo(), 15*time.Second)
		assert.False(t, connected)

		connected = connectToPeer(srv, peers[1].AddrInfo(), 15*time.Second)
		assert.True(t, connected)

		closeServers(srv, peers[1])
	})
}

func TestMinimumBootNodeCount(t *testing.T) {
	tests := []struct {
		name       string
		bootNodes  []string
		shouldFail bool
	}{
		{
			name:       "Server config with empty bootnodes",
			bootNodes:  []string{},
			shouldFail: true,
		},
		{
			name:       "Server config with less than two bootnodes",
			bootNodes:  []string{GenerateTestMultiAddr(t).String()},
			shouldFail: true,
		},
		{
			name:       "Server config with more than two bootnodes",
			bootNodes:  []string{GenerateTestMultiAddr(t).String(), GenerateTestMultiAddr(t).String()},
			shouldFail: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, createErr := CreateServer(&CreateServerParams{
				ConfigCallback: func(c *Config) {
					c.Chain.Bootnodes = tt.bootNodes
				},
				ServerCallback: nil,
				Logger:         nil,
			})

			if tt.shouldFail {
				assert.Error(t, createErr)
			} else {
				assert.NoError(t, createErr)
			}
			if createErr != nil {
				t.Fatalf("Unable to create server, %v", createErr)
			}
		})

	}
}
