package rlpx

import (
	"crypto/ecdsa"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
)

func TestRlpxListener(t *testing.T) {
	l, prv, close := listen(t)
	defer close()

	go func() {
		_, err := l.Accept()
		if err != nil {
			panic(err)
		}
	}()

	prv2, _ := crypto.GenerateKey()

	_, err := Dial("tcp", l.Addr().String(), &Config{Prv: prv2, Pub: &prv.PublicKey, Info: mockInfo(prv2)})
	if err != nil {
		t.Fatal(err)
	}
}

func listen(t *testing.T) (*Listener, *ecdsa.PrivateKey, func()) {
	prv, _ := crypto.GenerateKey()

	l, err := Listen("tcp", "127.0.0.1:0", &Config{Prv: prv, Info: mockInfo(prv)})
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
