package rlpx

import (
	"crypto/ecdsa"
	"fmt"
	"net"
	"reflect"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/umbracle/minimal/network/discover"
)

func TestP2PHandshake(t *testing.T) (*Session, *Session) {
	c0, c1, err := DoP2PHandshake()
	if err != nil {
		t.Fatal(err)
	}
	return c0, c1
}

func DoProtocolHandshake(c0 *Session, info0 *Info, c1 *Session, info1 *Info) error {
	errr := make(chan error, 2)

	go func() {
		info, err := StartProtocolHandshake(c0, info0)
		if err != nil {
			errr <- err
		} else if !reflect.DeepEqual(info.ID, info1.ID) { // reflect.DeepEqual(info, info1) does not seem to work
			errr <- fmt.Errorf("info not the same")
		} else {
			errr <- nil
		}
	}()

	go func() {
		info, err := StartProtocolHandshake(c1, info1)
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
		c0 = Server(conn0, prv0, nil)
		c0.LocalID = &prv0.PublicKey

		errs <- c0.p2pHandshake()
	}()

	go func() {
		c1 = Client(conn1, prv1, &prv0.PublicKey, nil)
		c1.LocalID = &prv0.PublicKey

		errs <- c1.p2pHandshake()
	}()

	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			return nil, nil, err
		}
	}

	return c0, c1, nil

	/*
		r0, r1 := <-res, <-res
		if r0.err != nil {
			return nil, nil, r0.err
		}
		if r1.err != nil {
			return nil, nil, r1.err
		}

		sec0, sec1 := r0.res, r1.res
		if !reflect.DeepEqual(sec0.EgressMAC, sec1.IngressMAC) {
			return nil, nil, fmt.Errorf("Egress MAC mismatch")
		}
		if !reflect.DeepEqual(sec0.IngressMAC, sec1.EgressMAC) {
			return nil, nil, fmt.Errorf("Ingress MAC mismatch")
		}
		if !reflect.DeepEqual(sec0.AES, sec1.AES) {
			return nil, nil, fmt.Errorf("AES cipher mismatch")
		}
		if !reflect.DeepEqual(sec0.MAC, sec1.MAC) {
			return nil, nil, fmt.Errorf("MAC cipher mismatch")
		}

		c0, _ := newSession(conn0, sec0)
		c1, _ := newSession(conn1, sec1)

		c0.LocalID, c0.RemoteID = &prv0.PublicKey, &prv1.PublicKey
		c1.LocalID, c1.RemoteID = &prv1.PublicKey, &prv0.PublicKey

		return c0, c1, nil
	*/
}

func DummyInfo(name string, id *ecdsa.PublicKey) *Info {
	return dummyInfo(name, id)
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
