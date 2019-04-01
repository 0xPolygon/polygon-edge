package rlpx

import (
	"bytes"
	"crypto/rand"
	"io/ioutil"
	"testing"
)

func readMsg(t *testing.T, s *Stream) *Message {
	var header Header
	header = make([]byte, HeaderSize)
	if _, err := s.Read(header); err != nil {
		t.Fatal(err)
	}

	size := header.Length()

	buf := make([]byte, size)
	if _, err := s.Read(buf); err != nil {
		t.Fatal(err)
	}

	return &Message{
		Code:    uint64(header.MsgType()),
		Size:    size,
		Payload: bytes.NewBuffer(buf),
	}
}

func TestStreamMessage(t *testing.T) {
	c0, c1 := pipe(t)
	defer c0.Close()
	defer c1.Close()

	s0 := c0.OpenStream(5, 10)
	s1 := c1.OpenStream(5, 10)

	var h Header

	go func() {
		h = make([]byte, HeaderSize)
		h.Encode(0x1, 0)

		if _, err := s0.Write(h); err != nil {
			t.Fatal(err)
		}
	}()

	msg := readMsg(t, s1)
	if msg.Code != 0x1 {
		t.Fatal("bad")
	}
}

func TestStreamMessageWithBody(t *testing.T) {
	c0, c1 := pipe(t)
	defer c0.Close()
	defer c1.Close()

	s0 := c0.OpenStream(5, 10)
	s1 := c1.OpenStream(5, 10)

	var h Header

	size := 6
	buf := make([]byte, size)
	rand.Read(buf[:])

	go func() {
		h = make([]byte, HeaderSize)
		h.Encode(0x5, 6)

		if _, err := s0.Write(h); err != nil {
			t.Fatal(err)
		}
		if _, err := s0.Write(buf); err != nil {
			t.Fatal(err)
		}
	}()

	msg := readMsg(t, s1)
	if msg.Code != 0x5 {
		t.Fatal("bad")
	}
	data, err := ioutil.ReadAll(msg.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, buf) {
		t.Fatal("bad")
	}
}
