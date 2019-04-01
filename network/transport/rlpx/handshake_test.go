package rlpx

import (
	"crypto/ecdsa"
	"fmt"
	"net"
	"reflect"
	"testing"

	"github.com/umbracle/minimal/helper/enode"

	"github.com/ethereum/go-ethereum/crypto"
)

func TestHandshakeP2P(t *testing.T) {
	testP2PHandshake(t)
}

func TestHandshakeDevP2P(t *testing.T) {
	c0, c1 := testP2PHandshake(t)

	info0 := mockInfo(c0.prv) // 0 is is the remoteid of 1
	info1 := mockInfo(c1.prv)

	if err := DoProtocolHandshake(c0, info0, c1, info1); err != nil {
		t.Fatal(err)
	}
}

func TestHandshakeDevP2PWrongID(t *testing.T) {
	c0, c1 := testP2PHandshake(t)

	info0 := mockInfo(c1.prv)
	info1 := mockInfo(c0.prv)

	err := DoProtocolHandshake(c0, info0, c1, info1)
	if err == nil {
		t.Fatal("bad")
	}
	if err.Error() != "Node ID does not match" {
		t.Fatal("Node ID should not match")
	}
}

func TestHandshakeDevP2PTimeout(t *testing.T) {
	c0, _ := testP2PHandshake(t)
	info0 := mockInfo(c0.prv)

	errr := make(chan error, 2)

	go func() {
		_, err := doProtocolHandshake(c0, info0)
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

func testP2PHandshake(t *testing.T) (*Session, *Session) {
	c0, c1, err := DoP2PHandshake()
	if err != nil {
		t.Fatal(err)
	}
	return c0, c1
}

func DoProtocolHandshake(c0 *Session, info0 *Info, c1 *Session, info1 *Info) error {
	errr := make(chan error, 2)

	go func() {
		info, err := doProtocolHandshake(c0, info0)
		if err != nil {
			errr <- err
		} else if !reflect.DeepEqual(info.ID, info1.ID) { // reflect.DeepEqual(info, info1) does not seem to work
			errr <- fmt.Errorf("info not the same")
		} else {
			errr <- nil
		}
	}()

	go func() {
		info, err := doProtocolHandshake(c1, info1)
		if err != nil {
			errr <- err
		} else if !reflect.DeepEqual(info.ID, info0.ID) {
			errr <- fmt.Errorf("info not the same")
		} else {
			errr <- nil
		}
	}()

	for i := 0; i < 2; i++ {
		if err := <-errr; err != nil {
			return err
		}
	}

	return nil
}

func DoP2PHandshake() (*Session, *Session, error) {
	prv0, _ := crypto.GenerateKey()
	prv1, _ := crypto.GenerateKey()

	conn0, conn1 := net.Pipe()

	type result struct {
		err error
		res Secrets
	}

	errs := make(chan error, 2)

	var c0, c1 *Session

	go func() {
		c0 = Server(nil, conn0, prv0, nil)
		c0.LocalID = &prv0.PublicKey

		errs <- c0.p2pHandshake()
	}()

	go func() {
		c1 = Client(nil, conn1, prv1, &prv0.PublicKey, nil)
		c1.LocalID = &prv0.PublicKey

		errs <- c1.p2pHandshake()
	}()

	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			return nil, nil, err
		}
	}

	return c0, c1, nil
}

func mockInfo(prv *ecdsa.PrivateKey) *Info {
	return &Info{
		Version:    1,
		Name:       "mock",
		ListenPort: 30303,
		Caps:       Capabilities{&Cap{"eth", 1}, &Cap{"par", 2}},
		ID:         enode.PubkeyToEnode(&prv.PublicKey),
	}
}
