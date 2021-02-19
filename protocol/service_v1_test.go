package protocol

import (
	"context"
	"io/ioutil"
	"math/big"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/0xPolygon/minimal/blockchain"
	"github.com/0xPolygon/minimal/protocol/proto"
	"github.com/0xPolygon/minimal/types"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
)

func TestServiceV1Watch(t *testing.T) {
	mockSub := blockchain.NewMockSubscription()

	s := &serviceV1{
		logger: hclog.NewNullLogger(),
		subs:   mockSub,
		store:  &mockBlockchain{},
	}
	go s.start()

	grpcServer := grpc.NewServer()
	proto.RegisterV1Server(grpcServer, s)

	dir, err := ioutil.TempDir("/tmp", "service-watch")
	if err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(dir, "socket")

	lis, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer lis.Close()

	go grpcServer.Serve(lis)

	conn, err := grpc.Dial(socket, grpc.WithInsecure(), grpc.WithDialer(func(addr string, timeout time.Duration) (net.Conn, error) {
		return net.DialTimeout("unix", addr, timeout)
	}))
	if err != nil {
		t.Fatal(err)
	}
	clt := proto.NewV1Client(conn)

	stream, err := clt.Watch(context.Background(), &empty.Empty{})
	if err != nil {
		t.Fatal(err)
	}

	// wait for the stream to be registered
	time.Sleep(500 * time.Millisecond)

	mockSub.Push(&blockchain.Event{
		NewChain: []*types.Header{
			{
				Hash:   types.StringToHash("1"),
				Number: 100,
			},
		},
		Difficulty: big.NewInt(100),
	})
	recv, err := stream.Recv()
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, recv.Number, int64(100))
	assert.Equal(t, recv.Difficulty, "100")
}
