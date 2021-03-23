package ibft

import (
	"context"

	"github.com/0xPolygon/minimal/consensus/ibft/proto"
	"github.com/golang/protobuf/ptypes/empty"
)

type grpcTransport struct {
	proto.UnimplementedIbftServer

	b *Backend2
}

func (g *grpcTransport) broadcast(msg *proto.MessageReq) {

}

func (g *grpcTransport) Message(ctx context.Context, req *proto.MessageReq) (*empty.Empty, error) {
	// read the message and send it to a queue
	return nil, nil
}
