package rlpx

import (
	"fmt"
	"reflect"
	"sort"
	"testing"
	"time"

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

func readMsgCh(conn *Connection) chan Message {
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

func testConn(c0, c1 *Connection, msgs []req) error {
	req0 := make(chan message, 2)
	req1 := make(chan message, 2)

	closeCh := make(chan bool)

	responses := make(chan *resp, 2)

	runConn := func(conn *Connection, requests chan message, id string) {
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
	c0, c1 := TestP2PHandshake(t)
	if err := testConn(c0, c1, connCases); err != nil {
		t.Fatal(err.Error())
	}
}

func TestSnappyConn(t *testing.T) {
	c0, c1 := TestP2PHandshake(t)

	c0.Snappy = true
	c1.Snappy = true

	if err := testConn(c0, c1, connCases); err != nil {
		t.Fatal(err.Error())
	}
}

func TestOnlyOneSnappyConn(t *testing.T) {
	c0, c1 := TestP2PHandshake(t)
	c0.Snappy = true

	if err := testConn(c0, c1, connCases); err == nil {
		t.Fatal("Only conn0 with snappy enabled, it should fail")
	}
}

func TestP2PHandshake2(t *testing.T) {
	// test p2p handshake in general
}

func TestProtocolHandshake2(t *testing.T) {
	// test protocol handshake in general
}

type timestamp struct {
	t    time.Duration
	code uint64
}

type timestamps []*timestamp

func (t *timestamps) Copy() timestamps {
	tt := []*timestamp{}
	for _, i := range *t {
		tt = append(tt, &timestamp{i.t, i.code})
	}
	return tt
}

func (t timestamps) Len() int           { return len(t) }
func (t timestamps) Swap(i, j int)      { t[i], t[j] = t[j], t[i] }
func (t timestamps) Less(i, j int) bool { return t[i].t.Nanoseconds() < t[j].t.Nanoseconds() }

func TestMultipleSessionHandlers(t *testing.T) {
	s := NewSession(0, nil)

	tt := timestamps{
		{1 * time.Second, 0x1},
		{500 * time.Millisecond, 0x2},
		{200 * time.Millisecond, 0x3},
		{700 * time.Millisecond, 0x4},
	}

	expected := tt.Copy()
	sort.Sort(expected)

	now := time.Now()

	ack := make(chan AckMessage, len(tt))
	for _, i := range tt {
		s.SetHandler(i.code, ack, i.t)
	}

	span := time.Duration(100 * time.Millisecond).Nanoseconds()

	for _, i := range expected {
		a := <-ack
		if a.Code != i.code {
			t.Fatal("bad")
		}

		elapsed := time.Now().Sub(now)

		d := elapsed.Nanoseconds() - i.t.Nanoseconds()
		if d < 0 {
			d = d * -1
		}

		if d > span {
			t.Fatal("bad")
		}
	}
}

func TestSessionHandlerUpdate(t *testing.T) {
	s := NewSession(0, nil)

	ack0 := make(chan AckMessage, 1)
	s.SetHandler(1, ack0, 500*time.Millisecond)

	ack1 := make(chan AckMessage, 1)
	s.SetHandler(1, ack1, 700*time.Millisecond)

	select {
	case <-ack1:
		// good. seconds handler updates the first one
	case <-ack0:
		t.Fatal("it should have been updated")
	case <-time.After(1 * time.Second):
		t.Fatal("bad")
	}
}
