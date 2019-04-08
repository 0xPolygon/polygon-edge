package rlpx

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"hash"
	"net"
	"reflect"
	"time"

	mrand "math/rand"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/ecies"
	"github.com/ethereum/go-ethereum/crypto/secp256k1"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/umbracle/minimal/helper/enode"

	"golang.org/x/crypto/sha3"
)

const (
	maxUint24 = ^uint32(0) >> 8

	sskLen = 16 // ecies.MaxSharedKeyLength(pubKey) / 2
	sigLen = 65 // elliptic S256
	pubLen = 64 // 512 bit pubkey in uncompressed representation without format byte
	shaLen = 32 // hash length (for nonce etc)

	authMsgLen  = sigLen + shaLen + pubLen + shaLen + 1
	authRespLen = pubLen + shaLen + 1

	eciesOverhead = 65 /* pubkey */ + 16 /* IV */ + 32 /* MAC */

	encAuthMsgLen  = authMsgLen + eciesOverhead  // size of encrypted pre-EIP-8 initiator handshake
	encAuthRespLen = authRespLen + eciesOverhead // size of encrypted pre-EIP-8 handshake reply
)

const (
	handshakeReadTimeout  = 5 * time.Second
	handshakeWriteTimeout = 5 * time.Second
)

var padSpace = make([]byte, 300)

type handshakeConn struct {
	conn   net.Conn
	local  *ecies.PrivateKey
	remote *ecies.PublicKey
}

func (c *handshakeConn) readMessage(m plainDecoder) ([]byte, error) {
	var plainSize uint16
	switch m.(type) {
	case *authMsgV4:
		plainSize = encAuthMsgLen
	case *authRespV4:
		plainSize = encAuthRespLen
	default:
		return nil, fmt.Errorf("internal: Unknown message")
	}

	buf, dec, plain, err := c.readBytes(plainSize)
	if err != nil {
		return nil, err
	}
	if plain {
		// pre-EIP-8
		m.decodePlain(dec)
	} else {
		// EIP-8
		s := rlp.NewStream(bytes.NewReader(dec), 0)
		if err := s.Decode(m); err != nil {
			return nil, err
		}
	}
	return buf, nil
}

func (c *handshakeConn) readBytes(plainSize uint16) ([]byte, []byte, bool, error) {
	c.conn.SetReadDeadline(time.Now().Add(handshakeReadTimeout))

	buf := make([]byte, plainSize)
	if _, err := c.conn.Read(buf); err != nil {
		return buf, nil, false, err
	}

	// pre-EIP-8
	if dec, err := c.local.Decrypt(buf, nil, nil); err == nil {
		return buf, dec, true, nil
	}

	// EIP-8
	prefix := buf[:2]
	size := binary.BigEndian.Uint16(prefix)
	if size < plainSize {
		return nil, nil, false, fmt.Errorf("size underflow, need at least %d bytes", plainSize)
	}

	buf2 := make([]byte, size-plainSize+2)
	if _, err := c.conn.Read(buf2); err != nil {
		return buf, nil, false, err
	}

	buf = append(buf, buf2...)
	dec, err := c.local.Decrypt(buf[2:], nil, prefix)
	if err != nil {
		return nil, nil, false, err
	}
	return buf, dec, false, err
}

func (c *handshakeConn) writeMessage(m plainDecoder) ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := rlp.Encode(buf, m); err != nil {
		return nil, err
	}

	// Encode EIP8
	pad := padSpace[:mrand.Intn(len(padSpace)-100)+100]
	buf.Write(pad)

	prefix := make([]byte, 2)
	binary.BigEndian.PutUint16(prefix, uint16(buf.Len()+eciesOverhead))

	enc, err := ecies.Encrypt(rand.Reader, c.remote, buf.Bytes(), nil, prefix)
	if err != nil {
		return nil, err
	}
	enc = append(prefix, enc...)

	c.conn.SetWriteDeadline(time.Now().Add(handshakeWriteTimeout))
	if _, err = c.conn.Write(enc); err != nil {
		return nil, err
	}
	return enc, nil
}

// handshakeState contains the state of the encryption handshake.
type handshakeState struct {
	conn     handshakeConn
	isClient bool

	initNonce       []byte
	respNonce       []byte
	randomPrivKey   *ecies.PrivateKey // ecdhe-random
	remoteRandomPub *ecies.PublicKey  // ecdhe-random-pubk
}

func handshakeServer(conn net.Conn, prv *ecdsa.PrivateKey) (s Secrets, err error) {
	state := handshakeState{
		conn: handshakeConn{
			conn:  conn,
			local: ecies.ImportECDSA(prv),
		},
	}

	// Read handshake message (authMsgV4)
	authMsg := new(authMsgV4)
	authPacket, err := state.conn.readMessage(authMsg)
	if err != nil {
		return s, err
	}

	// Handle auth message
	if err := state.handleAuthMsg(authMsg, prv); err != nil {
		return s, nil
	}

	// Build response message (authRespV4)
	authResp, err := state.makeAuthResp()
	if err != nil {
		return s, nil
	}
	authRespPacket, err := state.conn.writeMessage(authResp)
	if err != nil {
		return s, nil
	}
	return state.Secrets(authPacket, authRespPacket)
}

func handshakeClient(conn net.Conn, prv *ecdsa.PrivateKey, remoteID *ecdsa.PublicKey) (s Secrets, err error) {
	state := &handshakeState{
		isClient: true,
		conn: handshakeConn{
			conn:   conn,
			local:  ecies.ImportECDSA(prv),
			remote: ecies.ImportECDSAPublic(remoteID),
		},
	}

	// Build auth message (authMsgV4)
	authMsg, err := state.makeAuthMsg(prv)
	if err != nil {
		return s, err
	}
	authPacket, err := state.conn.writeMessage(authMsg)
	if err != nil {
		return s, err
	}

	// Read response message (authRespV4)
	authRespMsg := new(authRespV4)
	authRespPacket, err := state.conn.readMessage(authRespMsg)
	if err != nil {
		return s, err
	}
	if err := state.handleAuthResp(authRespMsg); err != nil {
		return s, err
	}
	return state.Secrets(authPacket, authRespPacket)
}

// staticSharedSecret returns a static shared secret
func (h *handshakeState) staticSharedSecret(prv *ecdsa.PrivateKey) ([]byte, error) {
	return ecies.ImportECDSA(prv).GenerateShared(h.conn.remote, sskLen, sskLen)
}

// makeAuthMsg creates the initiator handshake message.
func (h *handshakeState) makeAuthMsg(prv *ecdsa.PrivateKey) (*authMsgV4, error) {
	// Generate random initiator nonce.
	h.initNonce = make([]byte, shaLen)
	if _, err := rand.Read(h.initNonce); err != nil {
		return nil, err
	}
	// Generate random keypair to for ECDH.
	var err error
	h.randomPrivKey, err = ecies.GenerateKey(rand.Reader, crypto.S256(), nil)
	if err != nil {
		return nil, err
	}

	// Sign known message: static-shared-secret ^ nonce
	token, err := h.staticSharedSecret(prv)
	if err != nil {
		return nil, err
	}
	signed := xor(token, h.initNonce)
	signature, err := crypto.Sign(signed, h.randomPrivKey.ExportECDSA())
	if err != nil {
		return nil, err
	}

	msg := &authMsgV4{
		Version: 4,
	}
	copy(msg.Signature[:], signature)
	copy(msg.InitiatorPubkey[:], crypto.FromECDSAPub(&prv.PublicKey)[1:])
	copy(msg.Nonce[:], h.initNonce)

	return msg, nil
}

func (h *handshakeState) handleAuthResp(msg *authRespV4) (err error) {
	h.respNonce = msg.Nonce[:]
	h.remoteRandomPub, err = importPublicKey(msg.RandomPubkey[:])
	return err
}

// Secrets represents the Session Secrets
// which are negotiated during the encryption handshake.
type Secrets struct {
	RemoteID              *ecdsa.PublicKey
	AES, MAC              []byte
	EgressMAC, IngressMAC hash.Hash
	Token                 []byte
}

func (h *handshakeState) handleAuthMsg(msg *authMsgV4, prv *ecdsa.PrivateKey) error {
	// Import the remote identity.
	h.initNonce = msg.Nonce[:]

	remotePub, err := enode.NodeIDToPubKey(msg.InitiatorPubkey[:])
	if err != nil {
		return err
	}
	h.conn.remote = ecies.ImportECDSAPublic(remotePub)

	// Generate random keypair for ECDH.
	// If a private key is already set, use it instead of generating one (for testing).
	if h.randomPrivKey == nil {
		h.randomPrivKey, err = ecies.GenerateKey(rand.Reader, crypto.S256(), nil)
		if err != nil {
			return err
		}
	}

	// Check the signature.
	token, err := h.staticSharedSecret(prv)
	if err != nil {
		return err
	}
	signedMsg := xor(token, h.initNonce)
	remoteRandomPub, err := secp256k1.RecoverPubkey(signedMsg, msg.Signature[:])
	if err != nil {
		return err
	}
	h.remoteRandomPub, _ = importPublicKey(remoteRandomPub)
	return nil
}

func importPublicKey(pubKey []byte) (*ecies.PublicKey, error) {
	var pubKey65 []byte
	switch len(pubKey) {
	case 64:
		// add 'uncompressed key' flag
		pubKey65 = append([]byte{0x04}, pubKey...)
	case 65:
		pubKey65 = pubKey
	default:
		return nil, fmt.Errorf("invalid public key length %v (expect 64/65)", len(pubKey))
	}
	// TODO: fewer pointless conversions
	pub, err := crypto.UnmarshalPubkey(pubKey65)
	if err != nil {
		return nil, err
	}
	return ecies.ImportECDSAPublic(pub), nil
}

// Secrets is called after the handshake is completed.
func (h *handshakeState) Secrets(auth, authResp []byte) (Secrets, error) {
	ecdheSecret, err := h.randomPrivKey.GenerateShared(h.remoteRandomPub, sskLen, sskLen)
	if err != nil {
		return Secrets{}, err
	}

	// derive base Secrets from ephemeral key agreement
	sharedSecret := crypto.Keccak256(ecdheSecret, crypto.Keccak256(h.respNonce, h.initNonce))
	aesSecret := crypto.Keccak256(ecdheSecret, sharedSecret)
	s := Secrets{
		RemoteID: h.conn.remote.ExportECDSA(),
		AES:      aesSecret,
		MAC:      crypto.Keccak256(ecdheSecret, aesSecret),
	}

	// setup sha3 instances for the MACs
	mac1 := sha3.NewLegacyKeccak256()
	mac1.Write(xor(s.MAC, h.respNonce))
	mac1.Write(auth)

	mac2 := sha3.NewLegacyKeccak256()
	mac2.Write(xor(s.MAC, h.initNonce))
	mac2.Write(authResp)

	if h.isClient {
		s.EgressMAC, s.IngressMAC = mac1, mac2
	} else {
		s.EgressMAC, s.IngressMAC = mac2, mac1
	}

	return s, nil
}

func (h *handshakeState) makeAuthResp() (*authRespV4, error) {
	// Generate random nonce.
	h.respNonce = make([]byte, shaLen)
	if _, err := rand.Read(h.respNonce); err != nil {
		return nil, err
	}

	pub := h.randomPrivKey.PublicKey
	msg := &authRespV4{
		Version: 4,
	}
	copy(msg.Nonce[:], h.respNonce)
	copy(msg.RandomPubkey[:], elliptic.Marshal(pub.Curve, pub.X, pub.Y)[1:])

	return msg, nil
}

// -- PROTOCOL HANDSHAKE --

var protocolDeadline = 5 * time.Second

func doProtocolHandshake(conn *Session, nodeInfo *Info) (*Info, error) {
	errr := make(chan error, 2)
	var peerInfo *Info

	conn.SetDeadline(time.Now().Add(protocolDeadline))

	go func() {
		errr <- conn.WriteMsg(handshakeMsg, nodeInfo)
	}()

	go func() {
		info, err := readProtocolHandshake(conn)
		peerInfo = info
		errr <- err
	}()

	for i := 0; i < 2; i++ {
		select {
		case err := <-errr:
			if err != nil {
				return nil, err
			}
		case <-time.After(5 * time.Second):
			return nil, fmt.Errorf("handshake timeout")
		}
	}

	if peerInfo.Version >= snappyProtocolVersion {
		conn.Snappy = true
	}

	conn.SetDeadline(time.Time{})
	return peerInfo, nil
}

func readProtocolHandshake(conn *Session) (*Info, error) {
	msg, err := conn.ReadMsg()
	if err != nil {
		return nil, err
	}

	if msg.Size > baseProtocolMaxMsgSize {
		return nil, fmt.Errorf("message too big")
	}

	if msg.Code == discMsg {
		return nil, decodeDiscMsg(msg.Payload)
	}
	if msg.Code != handshakeMsg {
		return nil, fmt.Errorf("expected handshake, got %x", msg.Code)
	}

	var info Info
	if err := msg.Decode(&info); err != nil {
		return nil, err
	}

	if (info.ID == enode.ID{}) {
		return nil, DiscInvalidIdentity
	}
	if !reflect.DeepEqual(enode.PubkeyToEnode(conn.RemoteID), info.ID) {
		return nil, fmt.Errorf("Node ID does not match")
	}

	return &info, nil
}

func xor(one, other []byte) (xor []byte) {
	xor = make([]byte, len(one))
	for i := 0; i < len(one); i++ {
		xor[i] = one[i] ^ other[i]
	}
	return xor
}
