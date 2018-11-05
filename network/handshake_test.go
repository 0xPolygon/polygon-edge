package network

import (
	"crypto/ecdsa"
	"testing"

	"github.com/umbracle/minimal/network/discover"
)

func testP2PHandshake(t *testing.T) (*Connection, *Connection) {
	c0, c1, err := DoP2PHandshake()
	if err != nil {
		t.Fatal(err)
	}
	return c0, c1
}

func TestP2PHandshake(t *testing.T) {
	testP2PHandshake(t)
}

func dummyInfo(name string, id *ecdsa.PublicKey) *Info {
	return &Info{
		Version:    1,
		Name:       name,
		ListenPort: 30303,
		Caps:       Capabilities{&Cap{"eth", 1}, &Cap{"par", 2}},
		ID:         discover.PubkeyToNodeID(id),
	}
}

func TestProtocolHandshake(t *testing.T) {
	c0, c1 := testP2PHandshake(t)

	info0 := dummyInfo("info0", c0.LocalID) // 0 is is the remoteid of 1
	info1 := dummyInfo("info1", c1.LocalID)

	if err := DoProtocolHandshake(c0, info0, c1, info1); err != nil {
		t.Fatal(err)
	}
}

func TestProtocolHandshakeWrongID(t *testing.T) {
	c0, c1 := testP2PHandshake(t)

	info0 := dummyInfo("info0", c1.LocalID)
	info1 := dummyInfo("info1", c0.LocalID)

	err := DoProtocolHandshake(c0, info0, c1, info1)
	if err == nil {
		t.Fatal("bad")
	}
	if err.Error() != "Node ID does not match" {
		t.Fatal("Node ID should not match")
	}
}

func TestProtocolHandshakeTimeout(t *testing.T) {
	c0, _ := testP2PHandshake(t)
	info0 := dummyInfo("info0", c0.LocalID)

	errr := make(chan error, 2)

	go func() {
		_, err := startProtocolHandshake(c0, info0)
		errr <- err
	}()

	err := <-errr
	if err == nil {
		t.Fatal("bad")
	}
	if err.Error() != "handshake timeout" {
		t.Fatal("Handshake timeout failed")
	}
}
