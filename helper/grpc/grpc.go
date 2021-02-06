package grpc

import (
	"context"

	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/protocol"
	"google.golang.org/grpc"

	inet "github.com/libp2p/go-libp2p-net"
)

// Protocol is the GRPC-over-libp2p protocol.
const grpcProtocolID protocol.ID = "/grpc/0.0.1"

// GRPCProtocol is the GRPC-transported protocol handler.
type GRPCProtocol struct {
	ctx        context.Context
	stopCh     chan struct{}
	host       host.Host
	grpcServer *grpc.Server
	streamCh   chan inet.Stream
}

// NewGRPCProtocol attaches the GRPC protocol to a host.
func NewGRPCProtocol(ctx context.Context, host host.Host) *GRPCProtocol {
	grpcServer := grpc.NewServer()
	grpcProtocol := &GRPCProtocol{
		ctx:        ctx,
		host:       host,
		grpcServer: grpcServer,
		streamCh:   make(chan inet.Stream),
	}
	host.SetStreamHandler(grpcProtocolID, grpcProtocol.HandleStream)
	// Serve will not return until Accept fails, when the ctx is canceled.
	// go grpcServer.Serve(newGrpcListener(grpcProtocol))
	return grpcProtocol
}

func (p *GRPCProtocol) Serve() {
	go p.grpcServer.Serve(newGrpcListener(p))
}

// GetGRPCServer returns the grpc server.
func (p *GRPCProtocol) GetGRPCServer() *grpc.Server {
	return p.grpcServer
}

// HandleStream handles an incoming stream.
func (p *GRPCProtocol) HandleStream(stream inet.Stream) {
	select {
	case <-p.ctx.Done():
		return
	case p.streamCh <- stream:
	}
}
