package ibft2

import (
	"fmt"

	"github.com/0xPolygon/minimal/consensus/ibft2/proto"
	"github.com/0xPolygon/minimal/network/grpc"
	"github.com/0xPolygon/minimal/types"
)

var ibftProto = "/ibft/0.1"

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

func (g *grpcTransport) Gossip(target []types.Address, msg *proto.MessageReq) error {
	fmt.Println("__ GOSSIP __")

	return nil
}

func (g *grpcTransport) Listen() chan *proto.MessageReq {
	return nil
}
