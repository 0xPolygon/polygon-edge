package grpc

import (
	"net"

	"github.com/libp2p/go-libp2p-core/network"
	manet "github.com/multiformats/go-multiaddr-net"
)

// streamConn represents a net.Conn wrapped to be compatible with net.conn
type streamConn struct {
	network.Stream
}

// LocalAddr returns the local address.
func (c *streamConn) LocalAddr() net.Addr {
	addr, err := manet.ToNetAddr(c.Stream.Conn().LocalMultiaddr())
	if err != nil {
		return fakeLocalAddr()
	}
	return addr
}

// RemoteAddr returns the remote address.
func (c *streamConn) RemoteAddr() net.Addr {
	addr, err := manet.ToNetAddr(c.Stream.Conn().RemoteMultiaddr())
	if err != nil {
		return fakeRemoteAddr()
	}
	return addr
}

var _ net.Conn = &streamConn{}

// fakeLocalAddr returns a dummy local address.
func fakeLocalAddr() net.Addr {
	localIp := net.ParseIP("127.0.0.1")
	return &net.TCPAddr{IP: localIp, Port: 0}
}

// fakeRemoteAddr returns a dummy remote address.
func fakeRemoteAddr() net.Addr {
	remoteIp := net.ParseIP("127.1.0.1")
	return &net.TCPAddr{IP: remoteIp, Port: 0}
}
