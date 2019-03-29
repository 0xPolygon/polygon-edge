package rlpx

import (
	"crypto/rand"

	"github.com/ethereum/go-ethereum/crypto/ecies"
	"github.com/ethereum/go-ethereum/rlp"
)

type plainDecoder interface {
	decodePlain([]byte)
}

// RLPx v4 handshake auth (EIP-8).
type authMsgV4 struct {
	gotPlain bool // whether read packet had plain format.

	Signature       [sigLen]byte
	InitiatorPubkey [pubLen]byte
	Nonce           [shaLen]byte
	Version         uint

	// Ignore additional fields
	Rest []rlp.RawValue `rlp:"tail"`
}

// seal in pre-eip8 format
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
	RandomPubkey [pubLen]byte
	Nonce        [shaLen]byte
	Version      uint

	// Ignore additional fields
	Rest []rlp.RawValue `rlp:"tail"`
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
