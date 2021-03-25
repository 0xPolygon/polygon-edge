package grpc

import (
	"context"

	net "github.com/0xPolygon/minimal/network"
	"github.com/libp2p/go-libp2p-core/network"
	"google.golang.org/grpc"
)

var _ net.Protocol = &GrpcStream{}

type GrpcStream struct {
	ID string

	ctx      context.Context
	streamCh chan network.Stream

	grpcServer *grpc.Server
}

func NewGrpcStream(id string) *GrpcStream {
	return &GrpcStream{
		ID:         id,
		grpcServer: grpc.NewServer(),
	}
}

func (g *GrpcStream) GetID() string {
	return g.ID
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
