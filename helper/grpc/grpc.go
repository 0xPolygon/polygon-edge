package grpc

import (
	"context"

	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/protocol"
	"google.golang.org/grpc"
)

// Protocol is the GRPC-over-libp2p protocol.
const grpcProtocolID protocol.ID = "/grpc/0.0.1"

// GRPCProtocol is the GRPC-transported protocol handler.
type GRPCProtocol struct {
	ctx        context.Context
	stopCh     chan struct{}
	host       host.Host
	grpcServer *grpc.Server
	streamCh   chan network.Stream
}

// NewGRPCProtocol attaches the GRPC protocol to a host.
func NewGRPCProtocol(ctx context.Context, host host.Host) *GRPCProtocol {
	grpcServer := grpc.NewServer()
	grpcProtocol := &GRPCProtocol{
		ctx:        ctx,
		host:       host,
		grpcServer: grpcServer,
		streamCh:   make(chan network.Stream),
	}
	host.SetStreamHandler(grpcProtocolID, grpcProtocol.HandleStream)
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
func (p *GRPCProtocol) HandleStream(stream network.Stream) {
	select {
	case <-p.ctx.Done():
		return
	case p.streamCh <- stream:
	}
}
