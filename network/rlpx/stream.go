package rlpx

import (
	"io"
	"math/rand"
	"sync"
	"time"
)

type Stream struct {
	offset uint64
	length uint64

	conn Conn
	Msgs chan Message

	respLock sync.Mutex
	handlers map[uint64]*handler
	timer    *time.Timer
}

func NewStream(offset uint64, length uint64, conn Conn) *Stream {
	return &Stream{
		offset:   offset,
		length:   length,
		conn:     conn,
		Msgs:     make(chan Message, 10),
		respLock: sync.Mutex{},
		handlers: map[uint64]*handler{},
	}
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

func (s *Stream) RemoteAddr() string {
	return s.conn.RemoteAddr()
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
