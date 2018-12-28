package rlpx

import (
	"crypto/ecdsa"
	"fmt"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
)

func TestRlpxListener(t *testing.T) {
	l, prv, close := listen(t)
	defer close()

	fmt.Println(l)
	fmt.Println(prv)

	go func() {
		conn, err := l.Accept()
		if err != nil {
			panic(err)
		}

		fmt.Println(conn)
	}()

	prv2, _ := crypto.GenerateKey()

	conn1, err := Dial("tcp", l.Addr().String(), &Config{prv: prv2, pub: &prv.PublicKey, info: mockInfo(prv2)})
	if err != nil {
		t.Fatal(err)
	}

	fmt.Println(conn1)
}

func listen(t *testing.T) (*Listener, *ecdsa.PrivateKey, func()) {
	prv, _ := crypto.GenerateKey()

	l, err := Listen("tcp", "127.0.0.1:0", &Config{prv: prv, info: mockInfo(prv)})
	if err != nil {
		t.Fatal(err)
	}
	close := func() {
		if err := l.Close(); err != nil {
			t.Fatal(err)
		}
	}
	return l, prv, close
}
