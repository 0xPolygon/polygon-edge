package network

import (
	"crypto/ecdsa"
	"fmt"
	"log"
	"math/rand"
	"net"
	"os"
	"reflect"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/umbracle/minimal/network/rlpx"
)

func random(min, max int) int {
	return rand.Intn(max-min) + min
}

func TestServers() (*Server, *Server) {
	prv0, _ := crypto.GenerateKey()
	prv1, _ := crypto.GenerateKey()

	newServer := func(key *ecdsa.PrivateKey) *Server {
		config := DefaultConfig()
		port := -1

		for {
			if port == -1 {
				port = random(10000, 60000)
			}

			config.BindPort = port

			logger := log.New(os.Stderr, "", log.LstdFlags)
			s, err := NewServer("minimal-test", key, config, logger)
			if err == nil {
				return s
			}
		}
	}

	return newServer(prv0), newServer(prv1)
}

func DoProtocolHandshake(c0 *rlpx.Connection, info0 *rlpx.Info, c1 *rlpx.Connection, info1 *rlpx.Info) error {
	errr := make(chan error, 2)

	go func() {
		info, err := rlpx.StartProtocolHandshake(c0, info0)
		if err != nil {
			errr <- err
		} else if !reflect.DeepEqual(info.ID, info1.ID) { // reflect.DeepEqual(info, info1) does not seem to work
			errr <- fmt.Errorf("info not the same")
		} else {
			errr <- nil
		}
	}()

	go func() {
		info, err := rlpx.StartProtocolHandshake(c1, info1)
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

func DoP2PHandshake() (*rlpx.Connection, *rlpx.Connection, error) {
	prv0, _ := crypto.GenerateKey()
	prv1, _ := crypto.GenerateKey()

	conn0, conn1 := net.Pipe()

	type result struct {
		err error
		res rlpx.Secrets
	}

	res := make(chan result, 2)

	go func() {
		s, err := rlpx.DoEncHandshake(conn0, prv0, &prv1.PublicKey)
		if err != nil {
			res <- result{err: err}
		} else if !reflect.DeepEqual(s.RemoteID, &prv1.PublicKey) {
			res <- result{err: fmt.Errorf("bad")}
		} else {
			res <- result{res: s}
		}
	}()

	go func() {
		s, err := rlpx.DoEncHandshake(conn1, prv1, nil)
		if err != nil {
			res <- result{err: err}
		} else if !reflect.DeepEqual(s.RemoteID, &prv0.PublicKey) {
			res <- result{err: fmt.Errorf("bad")}
		} else {
			res <- result{res: s}
		}
	}()

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

	c0, _ := rlpx.NewConnection(conn0, sec0)
	c1, _ := rlpx.NewConnection(conn1, sec1)

	c0.LocalID, c0.RemoteID = &prv0.PublicKey, &prv1.PublicKey
	c1.LocalID, c1.RemoteID = &prv1.PublicKey, &prv0.PublicKey

	return c0, c1, nil
}
