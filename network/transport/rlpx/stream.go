package rlpx

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math/rand"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

type Stream struct {
	offset uint64
	length uint64

	conn *Session
	Msgs chan Message

	respLock sync.Mutex
	handlers map[uint64]*handler
	timer    *time.Timer

	recvBuf  *bytes.Buffer
	recvLock sync.Mutex

	recvNotifyCh chan struct{}

	readDeadline  atomic.Value // time.Time
	writeDeadline atomic.Value // time.Time

	header Header
}

func NewStream(offset uint64, length uint64, conn *Session) *Stream {
	s := &Stream{
		offset:       offset,
		length:       length,
		conn:         conn,
		Msgs:         make(chan Message, 10),
		respLock:     sync.Mutex{},
		handlers:     map[uint64]*handler{},
		recvNotifyCh: make(chan struct{}, 1),
	}
	s.readDeadline.Store(time.Time{})
	s.writeDeadline.Store(time.Time{})
	return s
}

// Consume consumes the handler if exists
func (s *Stream) Consume(code uint64, payload io.Reader) bool {
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

const (
	sizeOfType   = 2
	sizeOfLength = 4
	HeaderSize   = sizeOfType + sizeOfLength
)

type Header []byte

func (h Header) MsgType() uint16 {
	return binary.BigEndian.Uint16(h[0:2])
}

func (h Header) Length() uint32 {
	return binary.BigEndian.Uint32(h[2:6])
}

func (h Header) Encode(msgType uint16, length uint32) {
	binary.BigEndian.PutUint16(h[0:2], msgType)
	binary.BigEndian.PutUint32(h[2:6], length)
}

func (s *Stream) Write(b []byte) (int, error) {
	if s.header == nil {
		if len(b) != HeaderSize {
			return 0, fmt.Errorf("bad length, expected the header")
		}

		s.header = make([]byte, HeaderSize)
		copy(s.header[:], b[:])

		if s.header.Length() == 0 {
			b = nil
		} else {
			return len(b), nil
		}
	}

	size, code := s.header.Length(), s.header.MsgType()
	s.header = nil

	if len(b) != int(size) {
		return 0, fmt.Errorf("Expected message of length %d but found %d", size, len(b))
	}

	msg := Message{
		Code:    uint64(code) + s.offset,
		Size:    size,
		Payload: bytes.NewReader(b),
	}

	if err := s.conn.Write(msg); err != nil {
		return 0, err
	}

	return len(b), nil
}

func (s *Stream) WriteMsg(msgcode uint64, data ...interface{}) error {
	return s.conn.WriteMsg(msgcode+s.offset, data...)
}

func (s *Stream) ReadMsg() (Message, error) {
	msg := <-s.Msgs
	return msg, msg.Err
}

func (s *Stream) deliver(msg *Message) {
	if !s.Consume(msg.Code, msg.Payload) {
		s.Msgs <- *msg
	}
}

func (s *Stream) SetHandler(code uint64, ackCh chan AckMessage, duration time.Duration) {
	callback := func(payload io.Reader) {
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

func (s *Stream) Close() error {
	return s.conn.Close()
}

func (s *Stream) Read(b []byte) (n int, err error) {
	defer asyncNotify(s.recvNotifyCh)

START:

	// If there is no data available, block
	s.recvLock.Lock()

	if s.recvBuf == nil || s.recvBuf.Len() == 0 {
		s.recvLock.Unlock()
		goto WAIT
	}

	// Read bytes
	n, _ = s.recvBuf.Read(b)
	s.recvLock.Unlock()

	return n, err

WAIT:
	var timeout <-chan time.Time
	var timer *time.Timer
	readDeadline := s.readDeadline.Load().(time.Time)
	if !readDeadline.IsZero() {
		delay := readDeadline.Sub(time.Now())
		timer = time.NewTimer(delay)
		timeout = timer.C
	}

	select {
	case <-s.recvNotifyCh:
		if timer != nil {
			timer.Stop()
		}
		goto START
	case <-timeout:
		return 0, fmt.Errorf("timeout")
	}
}

// asyncNotify is used to signal a waiting goroutine
func asyncNotify(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

// readData is used to handle a data frame
func (s *Stream) readData(msg *Message) error {
	// Copy into buffer
	s.recvLock.Lock()

	if s.recvBuf == nil {
		// Allocate the receive buffer just-in-time to fit the full data frame.
		// This way we can read in the whole packet without further allocations.
		s.recvBuf = bytes.NewBuffer(make([]byte, 0, msg.Size+HeaderSize))
	}

	var h Header
	h = make([]byte, HeaderSize)
	h.Encode(uint16(msg.Code-uint64(s.offset)), msg.Size)

	if _, err := s.recvBuf.Write(h); err != nil {
		s.recvLock.Unlock()
		return err
	}

	if _, err := copyZeroAlloc(s.recvBuf, msg.Payload); err != nil {
		s.recvLock.Unlock()
		return err
	}

	s.recvLock.Unlock()

	// Unblock any readers
	asyncNotify(s.recvNotifyCh)
	return nil
}

func (s *Stream) RemoteAddr() net.Addr {
	return s.conn.conn.RemoteAddr()
}

func (s *Stream) LocalAddr() net.Addr {
	return s.conn.conn.LocalAddr()
}

func (s *Stream) SetDeadline(t time.Time) error {
	return nil
}

func (s *Stream) SetReadDeadline(t time.Time) error {
	return nil
}

func (s *Stream) SetWriteDeadline(t time.Time) error {
	return nil
}

func copyZeroAlloc(w io.Writer, r io.Reader) (int64, error) {
	vbuf := copyBufPool.Get()
	buf := vbuf.([]byte)
	n, err := io.CopyBuffer(w, r, buf)
	copyBufPool.Put(vbuf)
	return n, err
}

var copyBufPool = sync.Pool{
	New: func() interface{} {
		return make([]byte, 4096)
	},
}
