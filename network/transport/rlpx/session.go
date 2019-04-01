package rlpx

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdsa"
	"crypto/hmac"
	"errors"
	"fmt"
	"hash"
	"io"
	"io/ioutil"
	"net"
	"sync"
	"time"

	"github.com/umbracle/minimal/network/common"
	"github.com/umbracle/minimal/protocol"

	"github.com/umbracle/minimal/helper/enode"

	"github.com/armon/go-metrics"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/golang/snappy"
)

const (
	defaultPongTimeout  = 10 * time.Second
	defaultPingInterval = 5 * time.Second
)

// A Config structure is used to configure an Rlpx session.
type Config struct {
	Prv  *ecdsa.PrivateKey
	Pub  *ecdsa.PublicKey
	Info *Info
}

var (
	ErrStreamClosed = fmt.Errorf("session closed")
)

type sessionState int

const (
	sessionEstablished sessionState = iota
	sessionClosed
)

// Message is the p2p message
type Message struct {
	Code       uint64
	Size       uint32 // size of the paylod
	Payload    io.Reader
	ReceivedAt time.Time
	Err        error
}

func (msg Message) Decode(val interface{}) error {
	s := rlp.NewStream(msg.Payload, uint64(msg.Size))
	if err := s.Decode(val); err != nil {
		return fmt.Errorf("failed to decode rlp: %v", err)
	}
	return nil
}

// Session is the Session between peers (implements net.Conn)
type Session struct {
	id   string
	conn net.Conn

	// TODO, create
	rlpx *Rlpx

	config  *Config
	streams []*Stream

	Info       *Info
	remoteInfo *Info

	isClient bool

	enc cipher.Stream
	dec cipher.Stream

	macCipher  cipher.Block
	egressMAC  hash.Hash
	ingressMAC hash.Hash

	rmu *sync.Mutex
	wmu *sync.Mutex

	RemoteID *ecdsa.PublicKey
	LocalID  *ecdsa.PublicKey

	prv *ecdsa.PrivateKey
	pub *ecdsa.PublicKey

	Snappy bool

	// shutdown
	shutdown     bool
	shutdownErr  error
	shutdownCh   chan struct{}
	shutdownLock sync.Mutex

	// ping/pong
	pongTimeout *time.Timer

	// state
	state     sessionState
	stateLock sync.Mutex
}

func (s *Session) p2pHandshake() error {
	var secrets Secrets
	var err error

	if s.isClient {
		secrets, err = handshakeClient(s.conn, s.prv, s.pub)
	} else {
		secrets, err = handshakeServer(s.conn, s.prv)
	}
	if err != nil {
		return err
	}

	s.macCipher, err = aes.NewCipher(secrets.MAC)
	if err != nil {
		return err
	}
	encc, err := aes.NewCipher(secrets.AES)
	if err != nil {
		return err
	}

	iv := make([]byte, encc.BlockSize())
	s.enc = cipher.NewCTR(encc, iv)
	s.dec = cipher.NewCTR(encc, iv)

	s.egressMAC = secrets.EgressMAC
	s.ingressMAC = secrets.IngressMAC

	s.RemoteID = secrets.RemoteID
	s.id = enode.PubkeyToEnode(secrets.RemoteID).String()

	s.rmu = &sync.Mutex{}
	s.wmu = &sync.Mutex{}

	s.streams = []*Stream{}
	s.shutdownCh = make(chan struct{})
	s.shutdownLock = sync.Mutex{}

	// Set default values

	s.pongTimeout = time.NewTimer(defaultPongTimeout)

	s.stateLock = sync.Mutex{}
	s.state = sessionEstablished

	return nil
}

func (s *Session) RemoteIDString() string {
	return s.id
}

// Handshake does the p2p and protocol handshake
func (s *Session) Handshake() error {
	if err := s.p2pHandshake(); err != nil {
		return err
	}
	info, err := doProtocolHandshake(s, s.Info)
	if err != nil {
		return err
	}
	s.remoteInfo = info

	// start ping protocol and listen for incoming messages
	go s.keepalive()
	go s.recv()

	return nil
}

func (s *Session) RemoteInfo() *Info {
	return s.remoteInfo
}

// Disconnect sends a disconnect message to the peer and
// closes the session
func (s *Session) Disconnect(reason DiscReason) error {
	s.shutdownLock.Lock()
	defer s.shutdownLock.Unlock()

	if s.shutdown {
		return nil
	}
	s.shutdown = true
	if s.shutdownErr == nil {
		s.shutdownErr = reason
	}

	s.WriteMsg(discMsg, []DiscReason{reason})

	s.stateLock.Lock()
	s.state = sessionClosed
	s.stateLock.Unlock()

	s.shutdown = true
	close(s.shutdownCh)
	s.conn.Close()
	return nil
}

// LocalAddr implements the net.Conn interface
func (s *Session) LocalAddr() net.Addr {
	return s.conn.LocalAddr()
}

// RemoteAddr implements the net.Conn interface
func (s *Session) RemoteAddr() net.Addr {
	return s.conn.RemoteAddr()
}

// SetWriteDeadline implements the net.Conn interface
func (s *Session) SetWriteDeadline(t time.Time) error {
	return s.conn.SetWriteDeadline(t)
}

// SetReadDeadline implements the net.Conn interface
func (s *Session) SetReadDeadline(t time.Time) error {
	return s.conn.SetReadDeadline(t)
}

// SetDeadline implements the net.Conn interface
func (s *Session) SetDeadline(t time.Time) error {
	return s.conn.SetDeadline(t)
}

// Write implements the net.Conn interface
func (s *Session) Write(b []byte) (int, error) {
	return 0, fmt.Errorf("not implemented")
}

// Read implements the net.Conn interface
func (s *Session) Read(b []byte) (int, error) {
	return 0, fmt.Errorf("not implemented")
}

// Close implements the net.Conn interface
func (s *Session) Close() error {
	return s.Disconnect(DiscQuitting)
}

// NegociateProtocols implements the session interface
func (s *Session) NegociateProtocols(nInfo *common.Info) ([]*common.Instance, error) {
	info := networkInfoToLocalInfo(nInfo)

	offset := BaseProtocolLength
	// protocols := []*Instance{}

	type res struct { // will become matchProtocol struct in rlpx
		offset   uint64
		protocol protocol.Protocol
		backend  protocol.Backend
	}

	result := []*res{}

	for _, i := range info.Caps {
		// this one from the local instances
		if b := s.rlpx.getProtocol(i.Name, i.Version); b != nil {
			proto := b.Protocol()

			result = append(result, &res{
				backend:  b,
				protocol: proto,
				offset:   offset,
			})

			offset += proto.Length
		}
	}

	lock := sync.Mutex{}
	activated := []*common.Instance{}

	errr := make(chan error, len(result))
	for _, r := range result {
		go func(r *res) {
			stream := s.OpenStream(uint(r.offset), uint(r.protocol.Length))

			proto, err := r.backend.Add(stream, s.id)
			if err != nil {
				errr <- err
			}

			lock.Lock()
			activated = append(activated, &common.Instance{
				Protocol: r.protocol,
				Handler:  proto,
			})
			lock.Unlock()
			errr <- nil
		}(r)
	}

	for i := 0; i < len(result); i++ {
		if err := <-errr; err != nil {
			return nil, err
		}
	}
	return activated, nil
}

// GetInfo implements the session interface
func (s *Session) GetInfo() common.Info {
	info := common.Info{
		Client: s.remoteInfo.Name,
		Enode: &enode.Enode{
			ID: enode.PubkeyToEnode(s.RemoteID),
		},
	}
	return info
}

// CloseChan returns a read-only channel which is closed as
// soon as the session is closed.
func (s *Session) CloseChan() <-chan struct{} {
	return s.shutdownCh
}

// IsClosed does a safe check to see if we have shutdown
func (s *Session) IsClosed() bool {
	select {
	case <-s.shutdownCh:
		return true
	default:
		return false
	}
}

func (s *Session) recv() {
	if err := s.recvLoop(); err != nil {
		s.exitErr(err)
	}
}

func (s *Session) recvLoop() error {
	for {
		msg, err := s.ReadMsg()
		if err != nil {
			return err
		}

		// Reset timeout
		s.pongTimeout.Reset(defaultPongTimeout)

		switch {
		case msg.Code == pingMsg:
			if err := s.WriteMsg(pongMsg); err != nil {
				return err
			}

		case msg.Code == pongMsg:
			// Already handled

		case msg.Code == discMsg:
			msg := decodeDiscMsg(msg.Payload)

			fmt.Printf("DISCONNECTED: %s\n", msg.String())
			return msg
		default:
			s.handleStreamMessage(&msg)
		}
	}
}

func (s *Session) exitErr(err error) {
	s.shutdownLock.Lock()
	if s.shutdownErr == nil {
		s.shutdownErr = err
	}
	s.shutdownLock.Unlock()
	s.Close()
}

func (s *Session) keepalive() {
	for {
		select {
		case <-time.After(defaultPingInterval):
			if err := s.WriteMsg(pingMsg); err != nil {
				s.exitErr(err)
				return
			}

		case <-s.pongTimeout.C:
			s.exitErr(DiscProtocolError)
			return

		case <-s.shutdownCh:
			return
		}
	}
}

func (s *Session) OpenStream(offset uint, length uint) *Stream {
	ss := NewStream(uint64(offset), uint64(length), s)
	s.streams = append(s.streams, ss)
	return ss
}

func (s *Session) getStream(code uint64) *Stream {
	for _, proto := range s.streams {
		if code >= proto.offset && code < proto.offset+proto.length {
			return proto
		}
	}
	return nil
}

// ReadMsg from the Session
func (s *Session) ReadMsg() (msg Message, err error) {
	s.rmu.Lock()
	defer s.rmu.Unlock()

	// read the header
	headbuf := make([]byte, 32)
	if _, err := io.ReadFull(s.conn, headbuf); err != nil {
		return msg, err
	}
	// verify header mac
	shouldMAC := updateMAC(s.ingressMAC, s.macCipher, headbuf[:16])
	if !hmac.Equal(shouldMAC, headbuf[16:]) {
		return msg, errors.New("bad header MAC")
	}
	s.dec.XORKeyStream(headbuf[:16], headbuf[:16]) // first half is now decrypted
	fsize := readInt24(headbuf)
	// ignore protocol type for now

	// read the frame content
	var rsize = fsize // frame size rounded up to 16 byte boundary
	if padding := fsize % 16; padding > 0 {
		rsize += 16 - padding
	}
	framebuf := make([]byte, rsize)
	if _, err := io.ReadFull(s.conn, framebuf); err != nil {
		return msg, err
	}

	// read and validate frame MAC. we can re-use headbuf for that.
	s.ingressMAC.Write(framebuf)
	fmacseed := s.ingressMAC.Sum(nil)
	if _, err := io.ReadFull(s.conn, headbuf[:16]); err != nil {
		return msg, err
	}
	shouldMAC = updateMAC(s.ingressMAC, s.macCipher, fmacseed)
	if !hmac.Equal(shouldMAC, headbuf[:16]) {
		return msg, errors.New("bad frame MAC")
	}

	// decrypt frame content
	s.dec.XORKeyStream(framebuf, framebuf)

	// decode message code
	content := bytes.NewReader(framebuf[:fsize])
	if err := rlp.Decode(content, &msg.Code); err != nil {
		return msg, err
	}
	msg.Size = uint32(content.Len())
	msg.Payload = content

	// if snappy is enabled, verify and decompress message
	if s.Snappy {
		payload, err := ioutil.ReadAll(msg.Payload)
		if err != nil {
			return msg, err
		}
		size, err := snappy.DecodedLen(payload)
		if err != nil {
			return msg, err
		}
		if size > int(maxUint24) {
			return msg, errPlainMessageTooLarge
		}

		payload, err = snappy.Decode(nil, payload)
		if err != nil {
			return msg, err
		}
		msg.Size, msg.Payload = uint32(size), bytes.NewReader(payload)
	}

	metrics.SetGaugeWithLabels([]string{"conn", "inbound"}, float32(msg.Size), []metrics.Label{{Name: "id", Value: s.id}})
	return msg, nil
}

var (
	// this is used in place of actual frame header data.
	// TODO: replace this when Msg contains the protocol type code.
	zeroHeader = []byte{0xC2, 0x80, 0x80}
	// sixteen zero bytes
	zero16 = make([]byte, 16)
)

var errPlainMessageTooLarge = errors.New("message length >= 16MB")

func (s *Session) WriteMsg(msgcode uint64, input ...interface{}) error {
	var data interface{}

	l := len(input)
	if l == 0 {
		data = []interface{}{}
	} else if l == 1 {
		data = input[0]
	} else {
		panic("two messages not allowed")
	}

	size, r, err := rlp.EncodeToReader(data)
	if err != nil {
		return err
	}

	metrics.SetGaugeWithLabels([]string{"conn", "outbound"}, float32(size), []metrics.Label{{Name: "id", Value: s.id}})
	return s.WriteRawMsg(Message{Code: msgcode, Size: uint32(size), Payload: r})
}

func (s *Session) handleStreamMessage(msg *Message) {
	stream := s.getStream(msg.Code)
	stream.readData(msg)
}

func (s *Session) WriteRawMsg(msg Message) error {
	// check if the connection is open
	s.stateLock.Lock()
	defer s.stateLock.Unlock()

	if s.state == sessionClosed {
		return ErrStreamClosed
	}

	s.wmu.Lock()
	defer s.wmu.Unlock()

	ptype, err := rlp.EncodeToBytes(msg.Code)
	if err != nil {
		return err
	}

	// if snappy is enabled, compress message now
	if s.Snappy {
		if msg.Size > maxUint24 {
			return errPlainMessageTooLarge
		}
		payload, _ := ioutil.ReadAll(msg.Payload)
		payload = snappy.Encode(nil, payload)

		msg.Payload = bytes.NewReader(payload)
		msg.Size = uint32(len(payload))
	}

	// write header
	headbuf := make([]byte, 32)
	fsize := uint32(len(ptype)) + msg.Size
	if fsize > maxUint24 {
		return errors.New("message size overflows uint24")
	}

	putInt24(fsize, headbuf) // TODO: check overflow
	copy(headbuf[3:], zeroHeader)
	s.enc.XORKeyStream(headbuf[:16], headbuf[:16]) // first half is now encrypted

	// write header MAC
	copy(headbuf[16:], updateMAC(s.egressMAC, s.macCipher, headbuf[:16]))
	if _, err := s.conn.Write(headbuf); err != nil {
		return err
	}

	// write encrypted frame, updating the egress MAC hash with
	// the data written to conn.
	tee := cipher.StreamWriter{S: s.enc, W: io.MultiWriter(s.conn, s.egressMAC)}
	if _, err := tee.Write(ptype); err != nil {
		return err
	}
	if _, err := io.Copy(tee, msg.Payload); err != nil {
		return err
	}
	if padding := fsize % 16; padding > 0 {
		if _, err := tee.Write(zero16[:16-padding]); err != nil {
			return err
		}
	}

	// write frame MAC. egress MAC hash is up to date because
	// frame content was written to it as well.
	fmacseed := s.egressMAC.Sum(nil)
	mac := updateMAC(s.egressMAC, s.macCipher, fmacseed)
	if _, err := s.conn.Write(mac); err != nil {
		return err
	}

	return nil
}

// updateMAC reseeds the given hash with encrypted seed.
// it returns the first 16 bytes of the hash sum after seeding.
func updateMAC(mac hash.Hash, block cipher.Block, seed []byte) []byte {
	aesbuf := make([]byte, aes.BlockSize)
	block.Encrypt(aesbuf, mac.Sum(nil))
	for i := range aesbuf {
		aesbuf[i] ^= seed[i]
	}
	mac.Write(aesbuf)
	return mac.Sum(nil)[:16]
}

func readInt24(b []byte) uint32 {
	return uint32(b[2]) | uint32(b[1])<<8 | uint32(b[0])<<16
}

func putInt24(v uint32, b []byte) {
	b[0] = byte(v >> 16)
	b[1] = byte(v >> 8)
	b[2] = byte(v)
}
