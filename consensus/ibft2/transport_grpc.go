package ibft2

import (
	"fmt"

	"github.com/0xPolygon/minimal/consensus/ibft2/proto"
	"github.com/0xPolygon/minimal/network"
	"github.com/0xPolygon/minimal/network/grpc"
	"github.com/0xPolygon/minimal/types"
)

var ibftProto = "/ibft/0.1"

// TODO: As soon as you connect with them you have to exchange the keys
// and include it on the metadata

func grpcTransportFactory(ibft *Ibft2) (transport, error) {
	srv := &grpcTransport{ibft: ibft}

	grpc := grpc.NewGrpcStream()
	proto.RegisterIbftServer(grpc.GrpcServer(), srv)

	ibft.network.Register(ibftProto, grpc)

	// perform handshake after each node Connected
	err := ibft.network.SubscribeFn(func(evnt *network.PeerEvent) {
		panic("TODO")
	})
	if err != nil {
		return nil, err
	}

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
