package protocol2

import (
	"context"
	"testing"
	"time"

	libp2pgrpc "github.com/0xPolygon/minimal/network2/grpc"
	"github.com/0xPolygon/minimal/protocol2/proto"
	"github.com/0xPolygon/minimal/types"
	"github.com/umbracle/fastrlp"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/anypb"
)

func TestHeadWatcher(t *testing.T) {
	h := &headWatcher{}
	h.start()

	m := &mockHeadWatcher{}
	h.add(m)

	time.Sleep(1 * time.Second)

	m.Send(&types.Header{
		Number: 10,
	})

	time.Sleep(1 * time.Second)
}

type mockHeadWatcher struct {
	libp2pgrpc.MockGrpcClientStream
	recvCh chan *types.Header
}

func (m *mockHeadWatcher) Send(header *types.Header) {
	m.recvCh <- header
}

var defaultArena fastrlp.ArenaPool

func (m *mockHeadWatcher) Recv() (*proto.Component, error) {
	h := <-m.recvCh

	ar := defaultArena.Get()
	v := h.MarshalWith(ar)
	defer defaultArena.Put(ar)

	c := &proto.Component{
		Spec: &anypb.Any{
			Value: v.MarshalTo(nil),
		},
	}
	return c, nil
}

func (m *mockHeadWatcher) WatchBlocks(ctx context.Context, in *proto.Empty, opts ...grpc.CallOption) (proto.V1_WatchBlocksClient, error) {
	m.recvCh = make(chan *types.Header)
	return m, nil
}
