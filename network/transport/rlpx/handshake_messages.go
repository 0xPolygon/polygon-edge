package rlpx

import (
	"crypto/rand"
	"fmt"

	"github.com/umbracle/ecies"
	"github.com/umbracle/fastrlp"
)

var authMsgArenaPool fastrlp.ArenaPool

var authMsgParserPool fastrlp.ParserPool

// TODO, use fastrlp interface
type plainDecoder interface {
	decodePlain([]byte)
	UnmarshalRLP(b []byte) error
	MarshalRLP(dst []byte) []byte
}

// RLPx v4 handshake auth (EIP-8).
type authMsgV4 struct {
	gotPlain bool // whether read packet had plain format.

	Signature       []byte // sigLen
	InitiatorPubkey []byte // pubLen
	Nonce           []byte // shaLen
	Version         uint64 `rlp:"tail"`

	// Ignore additional fields
	// Rest []rlp.RawValue `rlp:"tail"`
}

func (a *authMsgV4) UnmarshalRLP(b []byte) error {
	p := authMsgParserPool.Get()
	defer authMsgParserPool.Put(p)

	v, err := p.Parse(b)
	if err != nil {
		return err
	}
	elems, err := v.GetElems()
	if err != nil {
		return err
	}
	if len(elems) < 4 {
		// there might be additional fields for forward compatibility
		return fmt.Errorf("bad")
	}

	a.Signature, err = elems[0].GetBytes(a.Signature, sigLen)
	if err != nil {
		return err
	}
	a.InitiatorPubkey, err = elems[1].GetBytes(a.InitiatorPubkey[:0], pubLen)
	if err != nil {
		return err
	}
	a.Nonce, err = elems[2].GetBytes(a.Nonce[:0], shaLen)
	if err != nil {
		return err
	}
	a.Version, err = elems[3].GetUint64()
	if err != nil {
		return err
	}
	return nil
}

func (a *authMsgV4) MarshalRLP(dst []byte) []byte {
	ar := authMsgArenaPool.Get()

	v := ar.NewArray()
	v.Set(ar.NewBytes(a.Signature))
	v.Set(ar.NewBytes(a.InitiatorPubkey))
	v.Set(ar.NewBytes(a.Nonce))
	v.Set(ar.NewUint(a.Version))

	dst = v.MarshalTo(dst)
	authMsgArenaPool.Put(ar)
	return dst
}

func (msg *authMsgV4) sealPlain(hs *handshakeState) ([]byte, error) {
	buf := make([]byte, authMsgLen)
	n := copy(buf, msg.Signature[:])
	n += shaLen
	n += copy(buf[n:], msg.InitiatorPubkey[:])
	copy(buf[n:], msg.Nonce[:])

	return ecies.Encrypt(rand.Reader, hs.conn.remote, buf, nil, nil)
}

func (msg *authMsgV4) decodePlain(input []byte) {
	n := copy(msg.Signature[:], input)
	n += shaLen // skip sha3(initiator-ephemeral-pubk)
	n += copy(msg.InitiatorPubkey[:], input[n:])
	copy(msg.Nonce[:], input[n:])
	msg.Version = 4
	msg.gotPlain = true
}

// RLPx v4 handshake response (EIP-8).
type authRespV4 struct {
	RandomPubkey []byte // pubLen
	Nonce        []byte // shaLen
	Version      uint64 `rlp:"tail"`

	// Ignore additional fields
	// Rest []rlp.RawValue `rlp:"tail"`
}

func (a *authRespV4) UnmarshalRLP(b []byte) error {
	p := authMsgParserPool.Get()
	defer authMsgParserPool.Put(p)

	v, err := p.Parse(b)
	if err != nil {
		return err
	}
	elems, err := v.GetElems()
	if err != nil {
		return err
	}
	if len(elems) < 3 {
		// there might be additional fields, ensure forward compatibility
		return fmt.Errorf("bad.1")
	}

	a.RandomPubkey, err = elems[0].GetBytes(a.RandomPubkey[:0], pubLen)
	if err != nil {
		return fmt.Errorf("failed to decode randompub: %v", err)
	}
	a.Nonce, err = elems[1].GetBytes(a.Nonce[:0], shaLen)
	if err != nil {
		return fmt.Errorf("failed to decode nonce: %v", err)
	}
	a.Version, err = elems[2].GetUint64()
	if err != nil {
		return fmt.Errorf("failed to decode version: %v", err)
	}

	return nil
}

func (a *authRespV4) MarshalRLP(dst []byte) []byte {
	ar := authMsgArenaPool.Get()

	v := ar.NewArray()
	v.Set(ar.NewBytes(a.RandomPubkey))
	v.Set(ar.NewBytes(a.Nonce))
	v.Set(ar.NewUint(a.Version))

	dst = v.MarshalTo(dst)
	authMsgArenaPool.Put(ar)
	return dst
}

func (msg *authRespV4) sealPlain(hs *handshakeState) ([]byte, error) {
	buf := make([]byte, authRespLen)
	n := copy(buf, msg.RandomPubkey[:])
	copy(buf[n:], msg.Nonce[:])
	return ecies.Encrypt(rand.Reader, hs.conn.remote, buf, nil, nil)
}

func (msg *authRespV4) decodePlain(input []byte) {
	n := copy(msg.RandomPubkey[:], input)
	copy(msg.Nonce[:], input[n:])
	msg.Version = 4
}
