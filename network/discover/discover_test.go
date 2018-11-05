package discover

import (
	"crypto/elliptic"
	"fmt"
	"math/rand"
	"net"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/ethereum/go-ethereum/crypto"
)

func pipe(config *Config) (*RoutingTable, *udpTransport, *RoutingTable, *udpTransport) {
	prv0, _ := crypto.GenerateKey()
	prv1, _ := crypto.GenerateKey()

	c0 := newUDPTransport()
	c1 := newUDPTransport()

	r0, err := NewRoutingTable(prv0, c0, c0.udpAddr, config)
	if err != nil {
		panic(err)
	}
	r1, err := NewRoutingTable(prv1, c1, c1.udpAddr, config)
	if err != nil {
		panic(err)
	}

	return r0, c0, r1, c1
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

			p, err := newPeer(id, nil)
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
	r0, _, r1, c1 := pipe(DefaultConfig())

	expiration := uint64(time.Now().Unix())

	r0.sendPacket(r1.local, pingPacket, pingRequest{
		Version:    4,
		From:       r0.local.toRPCEndpoint(),
		To:         r1.local.toRPCEndpoint(),
		Expiration: expiration,
	})

	time.Sleep(50 * time.Millisecond)

	p := <-c1.PacketCh()

	err := r1.HandlePacket(p)
	if err == nil {
		t.Fatal("bad")
	}
	if err.Error() != "ping: Message has expired" {
		t.Fatal("bad2")
	}
}

func TestNotExpectedPacket(t *testing.T) {
	r0, _, r1, c1 := pipe(DefaultConfig())

	r0.sendPacket(r1.local, pongPacket, pongResponse{
		To:         r1.local.toRPCEndpoint(),
		ReplyTok:   []byte{},
		Expiration: uint64(time.Now().Add(20 * time.Second).Unix()),
	})

	p := <-c1.PacketCh()

	err := r1.HandlePacket(p)
	if err == nil {
		t.Fatal("error expected")
	}
	if !strings.HasPrefix(err.Error(), "pongPacket not expected") {
		t.Fatal(err)
	}
}

func TestPingPong(t *testing.T) {
	r0, c0, r1, c1 := pipe(DefaultConfig())

	r0.sendPacket(r1.local, pingPacket, pingRequest{
		Version:    4,
		From:       r0.local.toRPCEndpoint(),
		To:         r1.local.toRPCEndpoint(),
		Expiration: uint64(time.Now().Add(10 * time.Second).Unix()),
	})

	p := <-c1.PacketCh()

	if err := r1.HandlePacket(p); err != nil {
		t.Fatal(err)
	}

	p = <-c0.PacketCh()

	_, sigdata, _, err := decodePacket(p.Buf)
	if err != nil {
		t.Fatal(err)
	}
	if sigdata[0] != pongPacket {
		t.Fatal("expected pong packet")
	}
}

func testProbeNode(t *testing.T, r0 *RoutingTable, c0 *udpTransport, r1 *RoutingTable, c1 *udpTransport) {
	// --- 0 probe starts ---

	// 0. send ping packet
	go r0.probeNode(r1.local)

	// 1. receive ping packet (send pong packet)
	p := <-c1.PacketCh()
	if err := r1.HandlePacket(p); err != nil {
		t.Fatal(err)
	}

	// 0. receive pong packet (finish 0. probe)
	p = <-c0.PacketCh()
	if err := r0.HandlePacket(p); err != nil {
		t.Fatal(err)
	}

	// r0 should have now r1 as a peer
	time.Sleep(200 * time.Millisecond)

	peers := r0.GetPeers()
	if len(peers) != 1 {
		t.Fatal("bad")
	}

	if !reflect.DeepEqual(peers[0].UDPAddr, c1.udpAddr) {
		t.Fatal("address of probe node is not the same")
	}
	if peers[0].Last.String() != p.Timestamp.String() {
		t.Fatal("timestamp of the peer is different")
	}

	// --- 1 probe starts ---

	// 0. receive ping packet
	p = <-c0.PacketCh()

	peer, err := decodePeerFromPacket(p)
	if err != nil {
		t.Fatal(err)
	}
	if peer.addr() != r1.local.addr() {
		t.Fatal("bad")
	}
	if r0.hasExpired(peer) == true {
		t.Fatal("peer should not have expired")
	}

	if err := r0.HandlePacket(p); err != nil {
		t.Fatal("bad")
	}

	// the timestamp of the peer should be updated
	peer1, ok := r0.getPeer(peer.ID)
	if !ok {
		t.Fatal("bad")
	}
	if peer1.Last.String() != p.Timestamp.String() {
		t.Fatal("timestamp of the peer has not been updated")
	}

	// 1.receive pong packet
	p = <-c1.PacketCh()
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
		t.Fatal("bad")
	}
	if peer1.Last.String() != p.Timestamp.String() {
		t.Fatal("timestamp of the peer has not been updated")
	}
}

func TestProbeNode(t *testing.T) {
	r0, c0, r1, c1 := pipe(DefaultConfig())
	testProbeNode(t, r0, c0, r1, c1)
}

func TestFindNodeWithUnavailableNode(t *testing.T) {
	config := DefaultConfig()
	config.RespTimeout = 5 * time.Second

	r0, c0, r1, c1 := pipe(config)

	testProbeNode(t, r0, c0, r1, c1)

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
	config.RespTimeout = 2 * time.Second

	var cases = []int{1, 5, 10, 15, 20, 30, 50}

	for _, cc := range cases {
		t.Run("", func(t *testing.T) {
			r0, c0, r1, c1 := pipe(config)
			testProbeNode(t, r0, c0, r1, c1)

			r0.Schedule()
			r1.Schedule()

			// r1. populate the buckets
			for i := 0; i < cc; i++ {
				prv, _ := crypto.GenerateKey()
				pub := &prv.PublicKey
				id := hexutil.Encode(elliptic.Marshal(pub.Curve, pub.X, pub.Y)[1:])

				addr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345}
				p, err := newPeer(id, addr)
				if err != nil {
					t.Fatal(err)
				}

				r1.updatePeer(p)
			}

			// r0. find node request
			nodes, err := r0.findNodes(r1.local, r0.local.Bytes)
			if err != nil {
				t.Fatal(err)
			}

			expected, err := r1.nearestPeers(r0.local.Bytes)
			if err != nil {
				t.Fatal(err)
			}

			if len(expected) != len(nodes) {
				t.Fatalf("length should be the same. Expected %d but found %d", len(expected), len(nodes))

				fmt.Println("--- SSS ---")
				fmt.Println(expected)
				fmt.Println(nodes)
				fmt.Println(r1.nearestPeers(r0.local.Bytes))
			}
			for indx := range expected {
				if !reflect.DeepEqual(expected[indx].ID, nodes[indx].ID) {
					t.Fatal("id not equal")
				}
			}

			// r0. all the expected nodes without a timestamp
			for _, n := range nodes {
				if n.ID == r1.local.ID { // this one has a timestamp from the probe
					continue
				}
				p, ok := r0.getPeer(n.ID)
				if !ok {
					t.Fatal("peer not found")
				}
				if p.Last != nil {
					t.Fatal("timestamp is set")
				}
			}

			// close()
		})
	}
}

const (
	udpPacketBufSize = 65536
	udpRecvBufSize   = 2 * 1024 * 1024
)

// udpTransport outside the transport code to test the discover protocol
type udpTransport struct {
	packetCh    chan *Packet
	udpListener *net.UDPConn
	udpAddr     *net.UDPAddr
}

func random(min, max int) int {
	rand.Seed(time.Now().Unix())
	return rand.Intn(max-min) + min
}

func newUDPTransport() *udpTransport {
	ip := net.ParseIP("127.0.0.1")

	var udpLn *net.UDPConn
	var udpAddr *net.UDPAddr
	var err error

	port := -1
	for {
		if port == -1 {
			port = random(10000, 60000)
		}

		udpAddr = &net.UDPAddr{IP: ip, Port: port}
		udpLn, err = net.ListenUDP("udp", udpAddr)

		if err != nil {
			port = -1
		} else {
			break
		}
	}

	if err := setUDPRecvBuf(udpLn); err != nil {
		panic(err)
	}

	t := &udpTransport{
		packetCh:    make(chan *Packet),
		udpListener: udpLn,
		udpAddr:     udpAddr,
	}

	go t.udpListen()
	return t
}

func setUDPRecvBuf(c *net.UDPConn) error {
	size := udpRecvBufSize
	var err error
	for size > 0 {
		if err = c.SetReadBuffer(size); err == nil {
			return nil
		}
		size = size / 2
	}
	return err
}

func (t *udpTransport) close() {
	if err := t.udpListener.Close(); err != nil {
		panic(err)
	}
}

func (t *udpTransport) udpListen() {
	for {
		buf := make([]byte, udpPacketBufSize)
		n, addr, err := t.udpListener.ReadFrom(buf)
		ts := time.Now()
		if err != nil {
			panic(err)
		}

		if n < 1 {
			panic("")
		}

		t.packetCh <- &Packet{
			Buf:       buf[:n],
			From:      addr,
			Timestamp: ts,
		}
	}
}

func (t *udpTransport) PacketCh() <-chan *Packet {
	return t.packetCh
}

func (t *udpTransport) WriteTo(b []byte, addr string) (time.Time, error) {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return time.Time{}, err
	}

	_, err = t.udpListener.WriteTo(b, udpAddr)
	return time.Now(), err
}
