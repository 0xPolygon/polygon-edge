package discv4

import (
	"crypto/elliptic"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/ethereum/go-ethereum/crypto"
)

func newTestDiscovery(config *Config, capturePacket bool) *Backend {
	logger := log.New(ioutil.Discard, "", log.LstdFlags)
	prv0, _ := crypto.GenerateKey()

PORT:
	rand.Seed(time.Now().Unix())
	port := rand.Intn(9000-5000) + 5000 // Random port between 5000 and 9000

	config.BindPort = port
	config.BindAddr = "127.0.0.1"

	r, err := NewBackend(logger, prv0, config)
	if err != nil {
		goto PORT
	}

	if capturePacket {
		r.packetCh = make(chan *Packet, 10)
	}

	return r
}

func pipe(config *Config, capturePacket bool) (*Backend, *Backend) {
	return newTestDiscovery(config, capturePacket), newTestDiscovery(config, capturePacket)
}

func TestPeerExpired(t *testing.T) {
	now := time.Now()

	var cases = []struct {
		Last    time.Time
		Now     time.Time
		Bond    time.Duration
		Expired bool
	}{
		{
			Last:    now.Add(1 * time.Second),
			Now:     now.Add(3 * time.Second),
			Bond:    1 * time.Second,
			Expired: true,
		},
		{
			Last:    now.Add(1 * time.Second),
			Now:     now.Add(3 * time.Second),
			Bond:    5 * time.Second,
			Expired: false,
		},
	}

	for _, cc := range cases {
		t.Run("", func(t *testing.T) {
			prv0, _ := crypto.GenerateKey()

			pub := &prv0.PublicKey
			id := hexutil.Encode(elliptic.Marshal(pub.Curve, pub.X, pub.Y)[1:])

			p, err := newPeer(id, nil, 0)
			if err != nil {
				t.Fatal(err)
			}

			p.Last = &cc.Last
			if p.hasExpired(cc.Now, cc.Bond) != cc.Expired {
				t.Fatal("bad")
			}
		})
	}
}

func TestExpiredPacket(t *testing.T) {
	r0, r1 := pipe(DefaultConfig(), true)

	expiration := uint64(time.Now().Unix())

	r0.sendPacket(r1.local, pingPacket, pingRequest{
		Version:    4,
		From:       r0.local.toRPCEndpoint(),
		To:         r1.local.toRPCEndpoint(),
		Expiration: expiration,
	})

	time.Sleep(50 * time.Millisecond)

	p := <-r1.packetCh

	err := r1.HandlePacket(p)
	if err == nil {
		t.Fatal("bad")
	}
	if err.Error() != "ping: Message has expired" {
		t.Fatal("bad2")
	}
}

func TestNotExpectedPacket(t *testing.T) {
	r0, r1 := pipe(DefaultConfig(), true)

	r0.sendPacket(r1.local, pongPacket, pongResponse{
		To:         r1.local.toRPCEndpoint(),
		ReplyTok:   []byte{},
		Expiration: uint64(time.Now().Add(20 * time.Second).Unix()),
	})

	p := <-r1.packetCh

	err := r1.HandlePacket(p)
	if err == nil {
		t.Fatal("error expected")
	}
	if !strings.HasPrefix(err.Error(), "pongPacket not expected") {
		t.Fatal(err)
	}
}

func TestPingPong(t *testing.T) {
	r0, r1 := pipe(DefaultConfig(), true)

	r0.sendPacket(r1.local, pingPacket, pingRequest{
		Version:    4,
		From:       r0.local.toRPCEndpoint(),
		To:         r1.local.toRPCEndpoint(),
		Expiration: uint64(time.Now().Add(10 * time.Second).Unix()),
	})

	p := <-r1.packetCh

	if err := r1.HandlePacket(p); err != nil {
		t.Fatal(err)
	}

	p = <-r0.packetCh

	_, sigdata, _, err := decodePacket(p.Buf)
	if err != nil {
		t.Fatal(err)
	}
	if sigdata[0] != pongPacket {
		t.Fatal("expected pong packet")
	}
}

func testProbeNode(t *testing.T, r0 *Backend, r1 *Backend) {
	// --- 0 probe starts ---
	// r0.Schedule()
	// r1.Schedule()

	// 0. send ping packet
	go r0.probeNode(r1.local)

	// 1. receive ping packet (send pong packet)
	p := <-r1.packetCh
	if err := r1.HandlePacket(p); err != nil {
		t.Fatal(err)
	}

	// 0. receive pong packet (finish 0. probe)
	p = <-r0.packetCh
	if err := r0.HandlePacket(p); err != nil {
		t.Fatal(err)
	}

	// r0 should have now r1 as a peer
	time.Sleep(200 * time.Millisecond)

	peers := r0.GetPeers()
	if len(peers) != 1 {
		t.Fatalf("Expected one peer but found %d", len(peers))
	}

	if !reflect.DeepEqual(peers[0].UDPAddr, r1.addr) {
		t.Fatal("address of probe node is not the same")
	}
	if peers[0].Last.String() != p.Timestamp.String() {
		t.Fatal("timestamp of the peer is different")
	}

	// --- 1 probe starts ---

	// 0. receive ping packet
	p = <-r0.packetCh

	peer, err := decodePeerFromPacket(p)
	if err != nil {
		t.Fatal(err)
	}
	if peer.addr() != r1.local.addr() {
		t.Fatalf("address mismatch: expected %s but found %s", r1.local.addr(), peer.addr())
	}
	if r0.hasExpired(peer) == true {
		t.Fatal("peer should not have expired")
	}

	if err := r0.HandlePacket(p); err != nil {
		t.Fatal(err)
	}

	// the timestamp of the peer should be updated
	peer1, ok := r0.getPeer(peer.ID)
	if !ok {
		t.Fatalf("peer %s not found after probe", peer.ID)
	}
	if peer1.Last.String() != p.Timestamp.String() {
		t.Fatal("timestamp of the peer has not been updated")
	}

	// 1.receive pong packet
	p = <-r1.packetCh
	if err := r1.HandlePacket(p); err != nil {
		t.Fatal(err)
	}

	peer, err = decodePeerFromPacket(p)
	if err != nil {
		t.Fatal(err)
	}

	// the timestamp of the peer should be updated
	peer1, ok = r1.getPeer(peer.ID)
	if !ok {
		t.Fatalf("peer %s not found after probe", peer.ID)
	}
	if peer1.Last.String() != p.Timestamp.String() {
		t.Fatal("timestamp of the peer has not been updated")
	}
}

func TestProbeNode(t *testing.T) {
	r0, r1 := pipe(DefaultConfig(), true)
	testProbeNode(t, r0, r1)
}

func TestFindNodeWithUnavailableNode(t *testing.T) {
	config := DefaultConfig()

	r0, r1 := pipe(config, true)

	testProbeNode(t, r0, r1)
	r0.packetCh, r1.packetCh = nil, nil

	r0.Schedule()
	r1.Close()

	_, err := r0.findNodes(r1.local, r0.local.Bytes)
	if err == nil {
		t.Fatal("it should fail")
	}
	if err.Error() != "failed to get peers" {
		t.Fatalf("bad error message: %v", err)
	}
}

func TestFindNode(t *testing.T) {
	config := DefaultConfig()

	var cases = []int{1, 5, 10, 15, 20, 30, 50}

	for _, cc := range cases {
		t.Run("", func(t *testing.T) {
			r0, r1 := pipe(config, true)

			testProbeNode(t, r0, r1)
			r0.packetCh, r1.packetCh = nil, nil

			// r1. populate the buckets
			for i := 0; i < cc; i++ {
				prv, _ := crypto.GenerateKey()
				pub := &prv.PublicKey
				id := hexutil.Encode(elliptic.Marshal(pub.Curve, pub.X, pub.Y)[1:])

				addr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345}
				p, err := newPeer(id, addr, 0)
				if err != nil {
					t.Fatal(err)
				}

				r1.updatePeer(p)
			}

			nodes, err := r0.findNodes(r1.local, r0.local.Bytes)
			if err != nil {
				t.Fatal(err)
			}
			expected, err := r1.NearestPeersFromTarget(r0.local.Bytes)
			if err != nil {
				t.Fatal(err)
			}

			if len(expected) != len(nodes) {
				t.Fatalf("length should be the same. Expected %d but found %d", len(expected), len(nodes))
			}
			for indx := range expected {
				if !reflect.DeepEqual(expected[indx].ID, nodes[indx].ID) {
					t.Fatalf("id not equal")
				}
			}
		})
	}
}
