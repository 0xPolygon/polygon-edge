package network

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdsa"
	"crypto/hmac"
	"errors"
	"hash"
	"io"
	"io/ioutil"
	"math/rand"
	"net"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/golang/snappy"
)

// AckMessage is the hook for the message handlers
type AckMessage struct {
	Complete bool
	Code     uint64 // only used for tests
	Payload  []byte
}

// Conn is the network connection
type Conn interface {
	WriteMsg(uint64, ...interface{}) error
	ReadMsg() (Message, error)
	SetHandler(code uint64, ackCh chan AckMessage, duration time.Duration)
	Close() error
}

// Message is the p2p message
type Message struct {
	Code       uint64
	Size       uint32 // size of the paylod
	Payload    []byte
	ReceivedAt time.Time
	Err        error
}

func (msg *Message) Copy() *Message {
	mm := new(Message)
	mm.Code = msg.Code
	mm.Size = msg.Size
	mm.Payload = msg.Payload
	return mm
}

func (msg Message) Decode(val interface{}) error {
	s := rlp.NewStream(bytes.NewReader(msg.Payload), uint64(msg.Size))
	if err := s.Decode(val); err != nil {
		panic(err)
	}
	return nil
}

type handler struct {
	callback func([]byte)
	salt     uint64
}

type Session struct {
	offset uint64
	conn   Conn
	msgs   chan Message

	respLock sync.Mutex
	handlers map[uint64]*handler
	timer    *time.Timer
}

func NewSession(offset uint64, conn Conn) *Session {
	return &Session{
		offset:   offset,
		conn:     conn,
		msgs:     make(chan Message, 10),
		respLock: sync.Mutex{},
		handlers: map[uint64]*handler{},
	}
}

// Consume consumes the handler if exists
func (s *Session) Consume(code uint64, payload []byte) bool {
	handler, ok := s.handlers[code]
	if !ok {
		return false
	}

	// consume the handler
	handler.callback(payload)

	s.respLock.Lock()
	delete(s.handlers, code)
	s.respLock.Unlock()

	return true
}

func (s *Session) WriteMsg(msgcode uint64, data ...interface{}) error {
	return s.conn.WriteMsg(msgcode+s.offset, data...)
}

func (s *Session) ReadMsg() (Message, error) {
	msg := <-s.msgs
	return msg, msg.Err
}

func (s *Session) SetHandler(code uint64, ackCh chan AckMessage, duration time.Duration) {
	callback := func(payload []byte) {
		select {
		case ackCh <- AckMessage{true, code, payload}:
		default:
		}
	}

	rand.Seed(time.Now().UnixNano())
	salt := rand.Uint64()

	handler := &handler{
		callback: callback,
		salt:     salt,
	}

	s.respLock.Lock()
	s.handlers[code] = handler
	s.respLock.Unlock()

	s.timer = time.AfterFunc(duration, func() {
		h, ok := s.handlers[code]
		if !ok {
			return
		}

		if h.salt != salt {
			return
		}

		s.respLock.Lock()
		delete(s.handlers, code)
		s.respLock.Unlock()

		select {
		case ackCh <- AckMessage{false, code, nil}:
		default:
		}
	})
}

func (s *Session) Close() error {
	return s.conn.Close()
}

// Connection is the connection between peers (implements net.Conn)
type Connection struct {
	conn net.Conn

	enc cipher.Stream
	dec cipher.Stream

	macCipher  cipher.Block
	egressMAC  hash.Hash
	ingressMAC hash.Hash

	rmu *sync.Mutex
	wmu *sync.Mutex

	RemoteID *ecdsa.PublicKey
	LocalID  *ecdsa.PublicKey

	Snappy bool
}

func newConnection(conn net.Conn, s Secrets) (*Connection, error) {
	macc, err := aes.NewCipher(s.MAC)
	if err != nil {
		return nil, err
	}
	encc, err := aes.NewCipher(s.AES)
	if err != nil {
		return nil, err
	}

	iv := make([]byte, encc.BlockSize())
	c := &Connection{
		conn:       conn,
		enc:        cipher.NewCTR(encc, iv),
		dec:        cipher.NewCTR(encc, iv),
		macCipher:  macc,
		egressMAC:  s.EgressMAC,
		ingressMAC: s.IngressMAC,
		rmu:        &sync.Mutex{},
		wmu:        &sync.Mutex{},
		RemoteID:   s.RemoteID,
	}

	return c, nil
}

// ReadMsg from the connection
func (c *Connection) ReadMsg() (msg Message, err error) {
	c.rmu.Lock()
	defer c.rmu.Unlock()

	// read the header
	headbuf := make([]byte, 32)
	if _, err := io.ReadFull(c.conn, headbuf); err != nil {
		return msg, err
	}
	// verify header mac
	shouldMAC := updateMAC(c.ingressMAC, c.macCipher, headbuf[:16])
	if !hmac.Equal(shouldMAC, headbuf[16:]) {
		return msg, errors.New("bad header MAC")
	}
	c.dec.XORKeyStream(headbuf[:16], headbuf[:16]) // first half is now decrypted
	fsize := readInt24(headbuf)
	// ignore protocol type for now

	// read the frame content
	var rsize = fsize // frame size rounded up to 16 byte boundary
	if padding := fsize % 16; padding > 0 {
		rsize += 16 - padding
	}
	framebuf := make([]byte, rsize)
	if _, err := io.ReadFull(c.conn, framebuf); err != nil {
		return msg, err
	}

	// read and validate frame MAC. we can re-use headbuf for that.
	c.ingressMAC.Write(framebuf)
	fmacseed := c.ingressMAC.Sum(nil)
	if _, err := io.ReadFull(c.conn, headbuf[:16]); err != nil {
		return msg, err
	}
	shouldMAC = updateMAC(c.ingressMAC, c.macCipher, fmacseed)
	if !hmac.Equal(shouldMAC, headbuf[:16]) {
		return msg, errors.New("bad frame MAC")
	}

	// decrypt frame content
	c.dec.XORKeyStream(framebuf, framebuf)

	// decode message code
	content := bytes.NewReader(framebuf[:fsize])
	if err := rlp.Decode(content, &msg.Code); err != nil {
		return msg, err
	}
	msg.Size = uint32(content.Len())

	payload, err := ioutil.ReadAll(content)
	if err != nil {
		return msg, err
	}

	msg.Payload = payload

	// if snappy is enabled, verify and decompress message
	if c.Snappy {
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
		msg.Size, msg.Payload = uint32(size), payload
	}
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

func (c *Connection) Close() error {
	return c.conn.Close()
}

func (c *Connection) WriteMsg(msgcode uint64, input ...interface{}) error {
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

	data2, err := ioutil.ReadAll(r)
	if err != nil {
		return err
	}

	return c.Write(Message{Code: msgcode, Size: uint32(size), Payload: data2})
}

func (c *Connection) SetHandler(code uint64, ackCh chan AckMessage, duration time.Duration) {
	panic("handlers not available in plain connection")
}

func (c *Connection) Write(msg Message) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()

	ptype, err := rlp.EncodeToBytes(msg.Code)
	if err != nil {
		return err
	}

	// if snappy is enabled, compress message now
	if c.Snappy {
		if msg.Size > maxUint24 {
			return errPlainMessageTooLarge
		}

		msg.Payload = snappy.Encode(nil, msg.Payload)
		msg.Size = uint32(len(msg.Payload))
	}

	// write header
	headbuf := make([]byte, 32)
	fsize := uint32(len(ptype)) + msg.Size
	if fsize > maxUint24 {
		return errors.New("message size overflows uint24")
	}

	putInt24(fsize, headbuf) // TODO: check overflow
	copy(headbuf[3:], zeroHeader)
	c.enc.XORKeyStream(headbuf[:16], headbuf[:16]) // first half is now encrypted

	// write header MAC
	copy(headbuf[16:], updateMAC(c.egressMAC, c.macCipher, headbuf[:16]))
	if _, err := c.conn.Write(headbuf); err != nil {
		return err
	}

	// write encrypted frame, updating the egress MAC hash with
	// the data written to conn.
	tee := cipher.StreamWriter{S: c.enc, W: io.MultiWriter(c.conn, c.egressMAC)}
	if _, err := tee.Write(ptype); err != nil {
		return err
	}
	if _, err := tee.Write(msg.Payload); err != nil {
		return err
	}
	if padding := fsize % 16; padding > 0 {
		if _, err := tee.Write(zero16[:16-padding]); err != nil {
			return err
		}
	}

	// write frame MAC. egress MAC hash is up to date because
	// frame content was written to it as well.
	fmacseed := c.egressMAC.Sum(nil)
	mac := updateMAC(c.egressMAC, c.macCipher, fmacseed)
	if _, err := c.conn.Write(mac); err != nil {
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
