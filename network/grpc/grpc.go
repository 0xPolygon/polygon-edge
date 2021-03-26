package grpc

import (
	"context"
	"io"
	"net"

	manet "github.com/multiformats/go-multiaddr-net"

	"github.com/libp2p/go-libp2p-core/network"
	"google.golang.org/grpc"
)

type GrpcStream struct {
	ctx      context.Context
	streamCh chan network.Stream

	grpcServer *grpc.Server
}

func NewGrpcStream() *GrpcStream {
	g := &GrpcStream{
		ctx:        context.Background(),
		streamCh:   make(chan network.Stream),
		grpcServer: grpc.NewServer(),
	}
	g.Serve()
	return g
}

func (g *GrpcStream) Serve() {
	go g.grpcServer.Serve(g)
}

func (g *GrpcStream) Handler() func(network.Stream) {
	return func(stream network.Stream) {
		select {
		case <-g.ctx.Done():
			return
		case g.streamCh <- stream:
		}
	}
}

func (g *GrpcStream) RegisterService(sd *grpc.ServiceDesc, ss interface{}) {
	g.grpcServer.RegisterService(sd, ss)
}

func (g *GrpcStream) GrpcServer() *grpc.Server {
	return g.grpcServer
}

// --- listener ---

func (g *GrpcStream) Accept() (net.Conn, error) {
	select {
	case <-g.ctx.Done():
		return nil, io.EOF
	case stream := <-g.streamCh:
		return &streamConn{Stream: stream}, nil
	}
}

// Addr implements the net.Listener interface
func (g *GrpcStream) Addr() net.Addr {
	return fakeLocalAddr()
}

func (g *GrpcStream) Close() error {
	return nil
}

// --- conn ---

func WrapClient(s network.Stream) *grpc.ClientConn {
	opts := grpc.WithContextDialer(func(ctx context.Context, peerIdStr string) (net.Conn, error) {
		return &streamConn{s}, nil
	})
	conn, err := grpc.Dial("", grpc.WithInsecure(), opts)
	if err != nil {
		// TODO: this should not fail at all
		panic(err)
	}
	return conn
}

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
