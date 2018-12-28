package network

import (
	"reflect"
	"testing"
	"time"

	"github.com/umbracle/minimal/network/rlpx"
	"github.com/umbracle/minimal/protocol"
)

func toCap(p protocol.Protocol) *rlpx.Cap {
	return &rlpx.Cap{Name: p.Name, Version: p.Version}
}

func TestMatchProtocols(t *testing.T) {
	type expected struct {
		name    string
		version uint
		offset  uint64
	}

	toExpected := func(c *rlpx.Cap, offset uint64) expected {
		return expected{name: c.Name, version: c.Version, offset: offset}
	}

	var cases = []struct {
		protocols []protocol.Protocol
		caps      rlpx.Capabilities
		expected  []expected
	}{
		{
			[]protocol.Protocol{
				protocol.ETH63,
			},
			[]*rlpx.Cap{
				toCap(protocol.ETH63),
			},
			[]expected{
				toExpected(toCap(protocol.ETH63), 16),
			},
		},
	}

	p := &Peer{}
	callback := func(session rlpx.Conn, peer *Peer) protocol.Handler {
		return nil
	}

	for _, cc := range cases {
		s := Server{Protocols: []*protocolStub{}}

		for _, p := range cc.protocols {
			s.RegisterProtocol(p, callback)
		}

		instances := s.matchProtocols(p, cc.caps)

		res := []expected{}
		for _, i := range instances {
			res = append(res, expected{i.protocol.Name, i.protocol.Version, i.offset})
		}

		if !reflect.DeepEqual(res, cc.expected) {
			t.Fatal("bad")
		}
	}
}

type dummyHandler struct{}

func (d *dummyHandler) Init() error {
	return nil
}

func (d *dummyHandler) Close() error {
	return nil
}

func testProtocolSessions(t *testing.T) (*Server, rlpx.Conn, *Server, rlpx.Conn) {
	s0, s1 := TestServers()

	// desactivate discover
	s0.discover.Close()
	s1.discover.Close()

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

	if err := s0.DialSync(s1.Enode); err != nil {
		t.Fatal(err)
	}

	time.Sleep(2000 * time.Millisecond)
	return s0, e0, s1, e1
}

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
	for _, v := range s0.peers {
		if v.Info.ID != s1.ID() {
			t.Fatal("s0 peer is bad")
		}
	}

	// s1 has s0 as a peer
	if len(s1.peers) != 1 {
		t.Fatal("s1 should have at least one peer")
	}
	for _, v := range s1.peers {
		if v.Info.ID != s0.ID() {
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
