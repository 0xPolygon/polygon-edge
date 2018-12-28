package network

import (
	"log"
	"os"
	"testing"
	"time"

	"github.com/umbracle/minimal/network/rlpx"
)

func testPeers(t *testing.T) (*Peer, *Peer) {
	c0, c1 := rlpx.TestP2PHandshake(t)

	info0 := rlpx.DummyInfo("info0", c0.LocalID)
	info1 := rlpx.DummyInfo("info1", c1.LocalID)

	if err := DoProtocolHandshake(c0, info0, c1, info1); err != nil {
		t.Fatal(err)
	}

	logger := log.New(os.Stderr, "", log.LstdFlags)
	return newPeer(logger, c0, info0, nil), newPeer(logger, c1, info1, nil)
}

func TestPing(t *testing.T) {
	p0, p1 := testPeers(t)

	go p0.Ping()

	msg, err := p1.conn.ReadMsg()
	if err != nil {
		t.Fatal(err)
	}
	if msg.Code != pingMsg {
		t.Fatal("ping packet expected")
	}
}

func TestPingPong(t *testing.T) {
	p0, p1 := testPeers(t)

	go p1.listen()

	// p0 sends ping message
	p0.Ping()

	// p1 sends pong message with listen
	// p0 receives pong packet
	msg, err := p0.conn.ReadMsg()
	if err != nil {
		t.Fatal(err)
	}
	if msg.Code != pongMsg {
		t.Fatal("pong packet expected")
	}
}

func TestPingInterval(t *testing.T) {
	p0, p1 := testPeers(t)

	p0.pingInterval = 1 * time.Second
	p0.pongTimeout = time.NewTimer(2 * time.Second)

	p0.Schedule()

	for i := 0; i < 2; i++ {
		msg, err := p1.conn.ReadMsg()
		if err != nil {
			t.Fatal(err)
		}
		if msg.Code != pingMsg {
			t.Fatal("expected ping msg")
		}
		p1.Pong()
	}

	// p0 should fail in 2 seconds
	time.Sleep(3 * time.Second)

	if p0.Connected == true {
		t.Fatal("It should be disconnected now")
	}
}

func TestPeerDisconnect(t *testing.T) {
	p0, p1 := testPeers(t)

	go p0.listen()

	if err := p1.conn.Close(); err != nil {
		t.Fatal(err)
	}

	time.Sleep(500 * time.Millisecond)
	if p0.Connected == true {
		t.Fatal("p0 is still connected")
	}
}

func TestDisconnectMsg(t *testing.T) {
	p0, p1 := testPeers(t)

	go p0.Disconnect(DiscTooManyPeers)

	msg, err := p1.conn.ReadMsg()
	if err != nil {
		t.Fatal(err)
	}
	if msg.Code != discMsg {
		t.Fatalf("expected discMsg %d but found %d", discMsg, msg.Code)
	}
	reason, err := decodeDiscMsg(msg.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if reason != DiscTooManyPeers {
		t.Fatalf("Reasons should be %d, instead %d found", DiscTooManyPeers, reason)
	}
}
