package duktape

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"unsafe"
)

type SocketTransport struct {
	ctx          *Context
	uData        interface{}
	requestFunc  DebugRequestFunc
	detachedFunc DebugDetachedFunc
	listener     net.Listener
	closed       bool
	conn         *connection
	m            sync.Mutex
}

type connection struct {
	conn          net.Conn
	reader        *bufio.Reader
	writer        *bufio.Writer
	errorListener func(err error)
}

func NewSocketTransport(ctx *Context,
	network, addr string,
	requestFunc DebugRequestFunc,
	detachedFunc DebugDetachedFunc,
	uData interface{}) (*SocketTransport, error) {

	listener, err := net.Listen(network, addr)
	if err != nil {
		return nil, err
	}

	return &SocketTransport{
		ctx:          ctx,
		uData:        uData,
		requestFunc:  requestFunc,
		detachedFunc: detachedFunc,
		listener:     listener,
		m:            sync.Mutex{},
	}, nil
}

func (s *SocketTransport) attach() error {
	debugger := DukDebugger()
	return debugger.Attach(
		s.ctx,
		s.conn.read,
		s.conn.write,
		s.conn.peek,
		nil,
		s.conn.writeFlush,
		s.requestFunc,
		s.detachedFunc,
		s.uData,
	)
}

func (s *SocketTransport) Close() error {
	s.m.Lock()
	defer s.m.Unlock()

	s.closed = true

	if s.conn != nil {
		if err := s.conn.conn.Close(); err != nil {
			return err
		}
	}

	return s.listener.Close()
}

func (s *SocketTransport) Listen(errorListener func(err error)) {
	acceptConnection := func(conn net.Conn) error {
		s.m.Lock()
		defer s.m.Unlock()

		if s.closed {
			return nil
		}

		if s.conn != nil {
			return errors.New("only one debugger can be connected at a time")
		}

		c := &connection{
			conn:          conn,
			reader:        bufio.NewReader(conn),
			writer:        bufio.NewWriter(conn),
			errorListener: errorListener,
		}

		s.conn = c
		if err := s.attach(); err != nil {
			return err
		}

		return nil
	}

	go func() {
		for {
			if s.closed {
				return
			}

			accepted, err := s.listener.Accept()
			if err != nil {
				if errorListener != nil {
					errorListener(err)
				}
				continue
			}

			if err := acceptConnection(accepted); err != nil {
				if errorListener != nil {
					errorListener(err)
				}
			}
		}
	}()
}

func (c *connection) read(uData unsafe.Pointer, buffer []byte) uint {
	r, err := io.ReadAtLeast(c.reader, buffer, 1)
	if err != nil {
		if err != io.EOF {
			if c.errorListener != nil {
				c.errorListener(errors.New(fmt.Sprintf("could not read from socket: %v", err)))
			}
		}
		return 0
	}
	return uint(r)
}

func (c *connection) write(uData unsafe.Pointer, buffer []byte) uint {
	w, err := c.writer.Write(buffer)
	if err != nil {
		if c.errorListener != nil {
			c.errorListener(errors.New(fmt.Sprintf("could not write to socket: %v", err)))
		}
		return 0
	}
	return uint(w)
}

func (c *connection) peek(uData unsafe.Pointer) uint {
	return uint(c.reader.Buffered())
}

func (c *connection) writeFlush(uData unsafe.Pointer) {
	if err := c.writer.Flush(); err != nil {
		if c.errorListener != nil {
			c.errorListener(errors.New(fmt.Sprintf("could not flush socket: %v", err)))
		}
	}
}
