package rlpx

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdsa"
	"crypto/hmac"
	"errors"
	"fmt"
	"hash"
	"io"
	"net"
	"sync"
	"time"

	"github.com/0xPolygon/minimal/helper/enode"
	"github.com/0xPolygon/minimal/network"
	"github.com/golang/snappy"
	"github.com/umbracle/fastrlp"
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

/*
func (msg Message) Decode(val interface{}) error {
	if err := rlp.DecodeReader(msg.Payload, val); err != nil {
		return fmt.Errorf("failed to decode rlp: %v", err)
	}
	return nil
}
*/

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
	enode      *enode.Enode

	isClient bool

	RemoteID *ecdsa.PublicKey
	LocalID  *ecdsa.PublicKey

	prv *ecdsa.PrivateKey
	pub *ecdsa.PublicKey

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

	in, out halfConn
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

	s.RemoteID = secrets.RemoteID
	s.id = enode.PubkeyToEnode(secrets.RemoteID).String()

	s.streams = []*Stream{}
	s.shutdownCh = make(chan struct{})
	s.shutdownLock = sync.Mutex{}

	// Set default values

	s.pongTimeout = time.NewTimer(defaultPongTimeout)

	s.stateLock = sync.Mutex{}
	s.state = sessionEstablished

	remoteAddr := "127.0.0.1:30303"
	if addr := s.RemoteAddr().String(); addr != "pipe" {
		remoteAddr = addr
	}

	enodeStr := fmt.Sprintf("enode://%s@%s", s.id, remoteAddr)
	enode, err := enode.ParseURL(enodeStr)
	if err != nil {
		return err
	}
	s.enode = enode

	macCipher, err := aes.NewCipher(secrets.MAC)
	if err != nil {
		return err
	}
	encc, err := aes.NewCipher(secrets.AES)
	if err != nil {
		return err
	}

	// tmp buffer for the CTR cipher. It gets cloned
	// inside so its safe to reuse it for both in/out
	tmp := make([]byte, encc.BlockSize())

	s.in = halfConn{
		conn:   s.conn,
		block:  macCipher,
		stream: cipher.NewCTR(encc, tmp),
		mac:    secrets.IngressMAC,
	}
	s.out = halfConn{
		conn:   s.conn,
		block:  macCipher,
		stream: cipher.NewCTR(encc, tmp),
		mac:    secrets.EgressMAC,
	}

	return nil
}

func (s *Session) SetSnappy() {
	s.in.snappy = true
	s.out.snappy = true
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

	// disconnect message is an array with one value
	s.WriteRawMsg(discMsg, []byte{0xc1, byte(reason)})
	// s.WriteMsg(discMsg, []DiscReason{reason})

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

// NegotiateProtocols implements the session interface
func (s *Session) negotiateProtocols() error {
	info := s.remoteInfo

	offset := BaseProtocolLength
	for _, i := range info.Caps {
		if b := s.rlpx.getProtocol(i.Name, uint(i.Version)); b != nil {
			s.OpenStream(uint(offset), uint(b.Spec.Length), b.Spec)
			offset += b.Spec.Length
		}
	}

	if len(s.streams) == 0 {
		return fmt.Errorf("no matching protocols")
	}
	return nil
}

// GetInfo implements the session interface
func (s *Session) GetInfo() network.Info {
	info := network.Info{
		Client: s.remoteInfo.Name,
		Enode:  s.enode,
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

var emptyRlp = []byte{0xC0}

func (s *Session) writeCode(code uint64) error {
	return s.WriteRawMsg(code, emptyRlp)
}

func (s *Session) recvLoop() error {
	for {
		code, buf, err := s.ReadMsg()
		if err != nil {
			return err
		}

		// Reset timeout
		s.pongTimeout.Reset(defaultPongTimeout)

		switch {
		case code == pingMsg:
			if err := s.writeCode(pongMsg); err != nil {
				return err
			}

		case code == pongMsg:
			// Already handled

		case code == discMsg:
			msg := decodeDiscMsg(buf)

			// TODO, logger
			return msg
		default:
			s.handleStreamMessage(code, buf)
		}
	}
}

func (s *Session) exitErr(err error) {
	s.shutdownLock.Lock()
	if s.shutdownErr == nil {
		s.shutdownErr = err
	}
	s.shutdownLock.Unlock()

	for _, i := range s.streams {
		select {
		case i.errorCh <- err:
		default:
		}
	}
	s.Close()
}

func (s *Session) keepalive() {
	for {
		select {
		case <-time.After(defaultPingInterval):
			if err := s.writeCode(pingMsg); err != nil {
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

// Streams implements the transport interface
func (s *Session) Streams() []network.Stream {
	b := make([]network.Stream, len(s.streams))
	for i := range s.streams {
		b[i] = s.streams[i]
	}
	return b
}

func (s *Session) OpenStream(offset uint, length uint, spec network.ProtocolSpec) *Stream {
	ss := NewStream(uint64(offset), uint64(length), s)
	ss.protocol = spec
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
func (s *Session) ReadMsg() (uint64, []byte, error) {
	s.in.Lock()
	defer s.in.Unlock()

	return s.in.read()
}

var errPlainMessageTooLarge = errors.New("message length >= 16MB")

/*
// TODO, remove
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

	buf, err := rlp.EncodeToBytes(data)
	if err != nil {
		return err
	}

	fmt.Println("-- msg code --")
	fmt.Println(msgcode)
	fmt.Println("-- input --")
	fmt.Println(input)
	fmt.Println("-- buf --")
	fmt.Println(buf)

	panic("X")

	// metrics.SetGaugeWithLabels([]string{"conn", "outbound"}, float32(size), []metrics.Label{{Name: "id", Value: s.id}})
	return s.WriteRawMsg(msgcode, buf)
}
*/

func (s *Session) handleStreamMessage(code uint64, buf []byte) {
	stream := s.getStream(code)
	stream.readData(code, buf)
}

func (s *Session) WriteRawMsg(code uint64, buf []byte) error {
	// check if the connection is open
	s.stateLock.Lock()
	defer s.stateLock.Unlock()

	if s.state == sessionClosed {
		return ErrStreamClosed
	}

	s.out.Lock()
	defer s.out.Unlock()

	return s.out.write(code, buf)
}

const (
	maxUint24 = 1<<24 - 1
)

var (
	empty = make([]byte, 16)
)

func extendByteSlice(b []byte, needLen int) []byte {
	b = b[:cap(b)]
	if n := needLen - cap(b); n > 0 {
		b = append(b, make([]byte, n)...)
	}
	return b[:needLen]
}

type halfConn struct {
	sync.Mutex

	conn  io.ReadWriter
	arena fastrlp.Arena

	snappy bool
	buf    []byte
	tmp    []byte

	aes    [aes.BlockSize]byte
	header [32]byte

	stream cipher.Stream
	block  cipher.Block
	mac    hash.Hash
}

func (s *halfConn) read() (uint64, []byte, error) {
	// read the header
	if _, err := io.ReadFull(s.conn, s.header[:]); err != nil {
		return 0, nil, err
	}

	// verify header mac (16..32)
	if !hmac.Equal(s.updateMacWithHeader(), s.header[16:]) {
		return 0, nil, fmt.Errorf("incorrect header mac")
	}
	// decrypt header
	s.stream.XORKeyStream(s.header[:16], s.header[:16])

	// Decode the size (24 bits)
	size := uint32(s.header[2]) | uint32(s.header[1])<<8 | uint32(s.header[0])<<16

	// Consider padding to decode full size
	fullSize := size
	if padding := size % 16; padding > 0 {
		fullSize += 16 - padding
	}

	s.buf = extendByteSlice(s.buf, int(fullSize))
	if _, err := io.ReadFull(s.conn, s.buf[:]); err != nil {
		return 0, nil, err
	}
	s.mac.Write(s.buf)

	// read payload mac
	if _, err := io.ReadFull(s.conn, s.header[:16]); err != nil {
		return 0, nil, err
	}

	// verify payload mac
	if !hmac.Equal(s.updateMac(), s.header[:16]) {
		return 0, nil, fmt.Errorf("incorrect payload mac")
	}
	// decrypt payload
	s.stream.XORKeyStream(s.buf, s.buf)

	// read the rlp code at the beginning. Note, since current eth63 messages are not
	// bigger than one byte, we only expect codes of one byte, otherwise it will return error.
	code, content, err := s.readCode(s.buf[:size])
	if err != nil {
		return 0, nil, err
	}

	if s.snappy {
		size, err := snappy.DecodedLen(content)
		if err != nil {
			return 0, nil, err
		}
		s.tmp = extendByteSlice(s.tmp, size)
		s.tmp, err = snappy.Decode(s.tmp, content)
		if err != nil {
			return 0, nil, fmt.Errorf("failed to decode snappy: %v", err)
		}
		content = s.tmp
	}
	return code, content, nil
}

// readCode in Rlp format
func (s *halfConn) readCode(buf []byte) (uint64, []byte, error) {
	cur := buf[0]
	if cur < 0x80 {
		return uint64(cur), buf[1:], nil
	}
	if cur < 0xB8 {
		size := int(cur - 0x80)
		if size > len(buf) {
			return 0, nil, fmt.Errorf("incorrect length")
		}
		if size == 0 {
			return 0, buf[1:], nil
		}
	}
	return 0, nil, fmt.Errorf("rlpx msg code is too big")
}

func (s *halfConn) write(code uint64, buf []byte) error {
	// encode the code as rlp. TODO, Remove once we do update in fastrlp
	v := s.arena.NewUint(code)
	ptype := v.MarshalTo(nil)
	s.arena.Reset()

	size := len(buf)
	var payload []byte

	// if snappy is enabled, compress message now
	if s.snappy {
		s.buf = extendByteSlice(s.buf, snappy.MaxEncodedLen(size))
		s.buf = snappy.Encode(s.buf, buf)

		size = len(s.buf)
		payload = s.buf
	} else {
		s.buf = extendByteSlice(s.buf, size)
		copy(s.buf, buf)
		payload = s.buf
	}

	fullSize := len(ptype) + size
	if fullSize > maxUint24 {
		return errors.New("message size overflows uint24")
	}

	// Encode the size as uint24
	s.header[0] = byte(fullSize >> 16)
	s.header[1] = byte(fullSize >> 8)
	s.header[2] = byte(fullSize)

	// Encode 'zero' header
	s.header[3] = 0xC0
	s.header[4] = 0x80
	s.header[5] = 0x80

	// first half is encrypted
	s.stream.XORKeyStream(s.header[:16], s.header[:16])

	// send the first half part of the header
	if _, err := s.conn.Write(s.header[:16]); err != nil {
		return err
	}
	// send the mac of the header (16 bytes)
	if _, err := s.conn.Write(s.updateMacWithHeader()); err != nil {
		return err
	}

	// write the payload
	if err := s.writeData(ptype); err != nil {
		return err
	}
	if err := s.writeData(payload); err != nil {
		return err
	}
	if padding := fullSize % 16; padding > 0 {
		if err := s.writeData(append([]byte{}, empty[:16-padding]...)); err != nil {
			return err
		}
	}

	// write the mac of the payload
	if _, err := s.conn.Write(s.updateMac()); err != nil {
		return err
	}
	return nil
}

func (s *halfConn) writeData(b []byte) error {
	// first make the xor
	s.stream.XORKeyStream(b, b)

	// write to connection
	if _, err := s.conn.Write(b); err != nil {
		return err
	}
	// write to mac
	if _, err := s.mac.Write(b); err != nil {
		return err
	}
	return nil
}

func (s *halfConn) updateMacWithHeader() []byte {
	return s.updateMacImpl(s.mac.Sum(nil), s.header[:16])
}

func (s *halfConn) updateMac() []byte {
	src := s.mac.Sum(nil)
	return s.updateMacImpl(src, src)
}

func (s *halfConn) updateMacImpl(src []byte, seed []byte) []byte {
	s.block.Encrypt(s.aes[:], src)
	for i := range s.aes {
		s.aes[i] ^= seed[i]
	}
	s.mac.Write(s.aes[:])
	return s.mac.Sum(nil)[:16]
}
