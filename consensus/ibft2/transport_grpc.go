package ibft2

import (
	"context"

	"github.com/0xPolygon/minimal/consensus/ibft2/proto"
	"github.com/0xPolygon/minimal/network/grpc"
	"github.com/golang/protobuf/ptypes/empty"
)

var ibftProto = "/ibft/0.1"

// TODO: As soon as you connect with them you have to exchange the keys
// and include it on the metadata

func grpcTransportFactory(ibft *Ibft2) (transport, error) {
	srv := &grpcTransport{ibft: ibft}

	grpc := grpc.NewGrpcStream()
	proto.RegisterIbftServer(grpc.GrpcServer(), srv)

	ibft.network.Register(ibftProto, grpc)

	return &grpcTransport{ibft: ibft}, nil
}

type grpcTransport struct {
	proto.UnimplementedIbftServer

	ibft *Ibft2
}

func (g *grpcTransport) Message(ctx context.Context, req *proto.MessageReq) (*empty.Empty, error) {
	return nil, nil
}

func (g *grpcTransport) Gossip(msg *proto.MessageReq) error {
	return nil
}

func (g *grpcTransport) Listen() chan *proto.MessageReq {
	return nil
}
