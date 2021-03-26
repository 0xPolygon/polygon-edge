package network

import (
	"context"
	"fmt"

	"github.com/0xPolygon/minimal/network/grpc"
	"github.com/0xPolygon/minimal/network/proto"
	"github.com/libp2p/go-libp2p-core/network"
)

var identityProtoV1 = "/id/0.1"

type identity struct {
	proto.UnimplementedIdentityServer

	srv *Server
}

func (i *identity) setup() {
	// register the protobuf protocol
	grpc := grpc.NewGrpcStream()
	proto.RegisterIdentityServer(grpc.GrpcServer(), i)

	i.srv.Register(identityProtoV1, grpc)

	// register callback messages to notify from new peers
	i.srv.host.Network().Notify(&network.NotifyBundle{
		ConnectedF: func(net network.Network, conn network.Conn) {
			go i.handleConnected(conn)
		},
	})
}

func (i *identity) handleConnected(conn network.Conn) {
	peerID := conn.RemotePeer()

	// we initiated the connection, perform handshake
	clt := proto.NewIdentityClient(grpc.WrapClient(i.srv.StartStream(identityProtoV1, peerID)))
	resp, err := clt.Hello(context.Background(), &proto.Status{})
	if err != nil {
		panic(err)
	}

	// validate resp
	if _, ok := resp.Metadata["a"]; !ok {
		// drop connection
	} else {
		// connection established
		i.srv.addPeer(peerID)
	}
}

func (i *identity) Hello(ctx context.Context, req *proto.Status) (*proto.Status, error) {
	fmt.Println(ctx.(*grpc.Context).PeerID)

	resp := &proto.Status{
		Metadata: map[string]string{
			"a": "b",
		},
	}
	return resp, nil
}
