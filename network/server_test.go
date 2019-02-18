package network

import (
	"testing"
	"time"

	"github.com/umbracle/minimal/network/rlpx"
	"github.com/umbracle/minimal/protocol"
)

func testProtocolSessions(t *testing.T) (*Server, rlpx.Conn, *Server, rlpx.Conn) {
	s0, s1 := TestServers(DefaultConfig())

	// sessions
	var e0, e1 rlpx.Conn

	p := protocol.Protocol{Name: "test", Version: 1, Length: 7}

	callback0 := func(session rlpx.Conn, peer *Peer) protocol.Handler {
		e0 = session
		return &dummyHandler{}
	}
	callback1 := func(session rlpx.Conn, peer *Peer) protocol.Handler {
		e1 = session
		return &dummyHandler{}
	}

	s0.RegisterProtocol(p, callback0)
	s1.RegisterProtocol(p, callback1)

	if err := s0.Schedule(); err != nil {
		t.Fatal(err)
	}
	if err := s1.Schedule(); err != nil {
		t.Fatal(err)
	}

	if err := s0.DialSync(s1.Enode.String()); err != nil {
		t.Fatal(err)
	}

	time.Sleep(2000 * time.Millisecond)
	return s0, e0, s1, e1
}

func toCap(p protocol.Protocol) *rlpx.Cap {
	return &rlpx.Cap{Name: p.Name, Version: p.Version}
}

/*
// Include once we decouple matchprotocol again
func TestMatchProtocols(t *testing.T) {
	convert := func(p protocol.Protocol, offset int) protocolMatch {
		return protocolMatch{Name: p.Name, Version: p.Version, Offset: offset}
	}

	var cases = []struct {
		Name     string
		Local    []protocol.Protocol
		Remote   rlpx.Capabilities
		Expected []protocolMatch
	}{
		{
			Name: "Only remote",
			Remote: []*rlpx.Cap{
				toCap(protocol.ETH63),
			},
		},
		{
			Name: "Only local",
			Local: []protocol.Protocol{
				protocol.ETH63,
			},
		},
		{
			Name: "Match",
			Local: []protocol.Protocol{
				protocol.ETH63,
			},
			Remote: []*rlpx.Cap{
				toCap(protocol.ETH63),
			},
			Expected: []protocolMatch{
				convert(protocol.ETH63, 5),
			},
		},
	}

	// p := &Peer{}
	callback := func(session rlpx.Conn, peer *Peer) protocol.Handler {
		return nil
	}

	for _, cc := range cases {
		t.Run(cc.Name, func(t *testing.T) {
			prv, _ := crypto.GenerateKey()
			s := Server{Protocols: []*protocolStub{}, key: prv}

			for _, p := range cc.Local {
				s.RegisterProtocol(p, callback)
			}
			s.buildInfo()

			res := s.matchProtocols2(cc.Remote)

			if i := len(res); i == len(cc.Expected) && i != 0 {
				if !reflect.DeepEqual(res, cc.Expected) {
					t.Fatal("bad")
				}
			}
		})
	}
}
*/

func TestSessionHandler(t *testing.T) {
	_, e0, _, e1 := testProtocolSessions(t)

	ack := make(chan rlpx.AckMessage, 1)
	e0.SetHandler(0x1, ack, 1*time.Second)

	if err := e1.WriteMsg(0x1, []byte{1, 2, 3}); err != nil {
		t.Fatal(err)
	}

	readOnce := func(conn rlpx.Conn) chan rlpx.Message {
		m := make(chan rlpx.Message, 5)
		go func() {
			msg, _ := conn.ReadMsg()
			m <- msg
		}()
		return m
	}

	select {
	case r := <-ack:
		if !r.Complete {
			t.Fatal("it should be completed")
		}
	case <-readOnce(e0):
		t.Fatal("it should be received on the handler not here")
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

func TestSessionHandlerTimeout(t *testing.T) {
	_, e0, _, _ := testProtocolSessions(t)

	ack := make(chan rlpx.AckMessage, 1)
	e0.SetHandler(0x1, ack, 1*time.Second)

	select {
	case r := <-ack:
		if r.Complete {
			t.Fatal("it should be incompleted")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

func TestSessionHandlerMessageAfterTimeout(t *testing.T) {
	_, e0, _, e1 := testProtocolSessions(t)

	ack := make(chan rlpx.AckMessage, 1)
	e0.SetHandler(0x1, ack, 600*time.Millisecond)

	time.Sleep(600 * time.Millisecond)

	if err := e1.WriteMsg(0x1, []byte{1, 2, 3}); err != nil {
		t.Fatal(err)
	}

	// e0 should receive the message from readMsg
	msg, err := e0.ReadMsg()
	if err != nil {
		t.Fatal(err)
	}
	if msg.Code != 0x1 {
		t.Fatal("expected 0x1")
	}
}

func TestProtocolEncoding(t *testing.T) {
	s0, e0, s1, e1 := testProtocolSessions(t)

	// s0 has s1 as a peer
	if len(s0.peers) != 1 {
		t.Fatal("s0 should have at least one peer")
	}
	for id := range s0.peers {
		if id != s1.ID().String() {
			t.Fatal("s0 peer is bad")
		}
	}

	// s1 has s0 as a peer
	if len(s1.peers) != 1 {
		t.Fatal("s1 should have at least one peer")
	}
	for id := range s1.peers {
		if id != s0.ID().String() {
			t.Fatal("s1 peer is bad")
		}
	}

	// send messages
	go e0.WriteMsg(0x1)

	msg, err := e1.ReadMsg()
	if err != nil {
		t.Fatal(err)
	}
	if msg.Code != 0x1 {
		t.Fatal("expected another msg")
	}
}
