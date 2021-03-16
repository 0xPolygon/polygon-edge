package peerstream_multiplex

import (
	"github.com/libp2p/go-libp2p-core/mux"
	mp "github.com/libp2p/go-mplex"
)

type conn mp.Multiplex

func (c *conn) Close() error {
	return c.mplex().Close()
}

func (c *conn) IsClosed() bool {
	return c.mplex().IsClosed()
}

// OpenStream creates a new stream.
func (c *conn) OpenStream() (mux.MuxedStream, error) {
	return c.mplex().NewStream()
}

// AcceptStream accepts a stream opened by the other side.
func (c *conn) AcceptStream() (mux.MuxedStream, error) {
	return c.mplex().Accept()
}

func (c *conn) mplex() *mp.Multiplex {
	return (*mp.Multiplex)(c)
}

var _ mux.MuxedConn = &conn{}
