package rlpx

import (
	"fmt"
	"net"
	"reflect"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
)

const (
	c0 = "c0"
	c1 = "c1"
)

type message struct {
	code    uint64
	payload []byte
}

type req struct {
	id  string
	msg message
}

type resp struct {
	id  string
	msg message
	err error
}

func readMsgCh(conn *Session) chan Message {
	msgs := make(chan Message, 10)
	go func() {
		for {
			msg, err := conn.ReadMsg()
			if err != nil {
				msg.Err = err
			}
			msgs <- msg
		}
	}()
	return msgs
}

func testConn(c0, c1 *Session, msgs []req) error {
	req0 := make(chan message, 2)
	req1 := make(chan message, 2)

	closeCh := make(chan bool)

	responses := make(chan *resp, 2)

	runConn := func(conn *Session, requests chan message, id string) {
		// con0
		go func() {
			msgs := readMsgCh(conn)
			for {
				select {
				case req := <-requests:
					if err := conn.WriteMsg(req.code, req.payload); err != nil {
						panic(err)
					}
				case msg := <-msgs:
					r := &resp{id: id}

					if msg.Err != nil {
						r.err = msg.Err
					} else {
						var payload []byte
						if err := rlp.Decode(msg.Payload, &payload); err != nil {
							r.err = err
						} else {
							r.msg = message{code: msg.Code, payload: payload}
						}
					}
					responses <- r

				case <-closeCh:
					return
				}
			}
		}()
	}

	runConn(c0, req0, "c0")
	runConn(c1, req1, "c1")

	for _, req := range msgs {
		if req.id == "c0" {
			req0 <- req.msg
		} else {
			req1 <- req.msg
		}

		r := <-responses

		if r.err != nil {
			return r.err
		}
		if req.id == r.id {
			return fmt.Errorf("packet received by the wrong client")
		}
		if req.msg.code != r.msg.code {
			return fmt.Errorf("msgcode is different")
		}
		if !reflect.DeepEqual(req.msg.payload, r.msg.payload) {
			return fmt.Errorf("payload is different")
		}
	}

	close(closeCh)
	return nil
}

var connCases = []req{
	{"c0", message{code: 0x1, payload: []byte{2, 3, 5}}},
	{"c0", message{code: 0x5, payload: []byte{1, 7, 8, 3, 4}}},
	{"c1", message{code: 0x10, payload: []byte{9, 9, 9, 9, 9}}},
}

func TestNonSnappyConn(t *testing.T) {
	c0, c1 := testP2PHandshake(t)
	if err := testConn(c0, c1, connCases); err != nil {
		t.Fatal(err.Error())
	}
}

func TestSnappyConn(t *testing.T) {
	c0, c1 := testP2PHandshake(t)

	c0.Snappy = true
	c1.Snappy = true

	if err := testConn(c0, c1, connCases); err != nil {
		t.Fatal(err.Error())
	}
}

func TestOnlyOneSnappyConn(t *testing.T) {
	c0, c1 := testP2PHandshake(t)
	c0.Snappy = true

	if err := testConn(c0, c1, connCases); err == nil {
		t.Fatal("Only conn0 with snappy enabled, it should fail")
	}
}

func pipe(t *testing.T) (*Session, *Session) {
	conn0, conn1 := net.Pipe()

	prv0, _ := crypto.GenerateKey()
	prv1, _ := crypto.GenerateKey()

	errs := make(chan error, 2)
	var c0, c1 *Session

	go func() {
		c0 = Server(nil, conn0, prv0, mockInfo(prv0))
		errs <- c0.Handshake()
	}()
	go func() {
		c1 = Client(nil, conn1, prv1, &prv0.PublicKey, mockInfo(prv1))
		errs <- c1.Handshake()
	}()

	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	return c0, c1
}

func TestPeerDisconnect(t *testing.T) {
	s0, s1 := pipe(t)

	if err := s1.Close(); err != nil {
		t.Fatal(err)
	}

	time.Sleep(500 * time.Millisecond)
	if !s0.IsClosed() {
		t.Fatal("p0 is still connected")
	}
}

func TestSessionDisconnect(t *testing.T) {
	c0, c1 := pipe(t)
	defer c1.Close()

	ch := c1.CloseChan()
	if err := c0.Disconnect(DiscTooManyPeers); err != nil {
		t.Fatalf("Failed to send disconnect message: %v", err)
	}

	select {
	case <-ch:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("failed")
	}
	if c1.shutdownErr != DiscTooManyPeers {
		t.Fatalf("Shutdown error should be DiscTooManyPeers but found %v", c1.shutdownErr)
	}
}

func TestSessionClose(t *testing.T) {
	c0, c1 := pipe(t)
	defer c1.Close()

	ch := c1.CloseChan()
	time.Sleep(100 * time.Millisecond)

	if err := c0.Close(); err != nil {
		t.Fatalf("Failed to send disconnect message: %v", err)
	}

	select {
	case <-ch:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("failed")
	}
	if c1.shutdownErr != DiscQuitting {
		t.Fatalf("Shutdown error should be DiscQuitting but found %v", c1.shutdownErr)
	}
}

func TestSessionWriteClosedConnection(t *testing.T) {
	c0, c1 := pipe(t)
	defer c1.Close()

	if err := c0.Close(); err != nil {
		t.Fatalf("Failed to send disconnect message: %v", err)
	}

	c1.CloseChan() // wait till is closed
	time.Sleep(100 * time.Millisecond)

	if err := c1.WriteMsg(0x10); err != ErrStreamClosed {
		t.Fatalf("It should be ErrStreamClosed but found %v", err)
	}
}

func TestSessionPingPong(t *testing.T) {
	t.Skip()
}
