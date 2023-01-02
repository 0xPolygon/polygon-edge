package syncer

import (
	"context"
	"io"
	"log"
	"net"
	"testing"

	"github.com/0xPolygon/polygon-edge/syncer/proto"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/emptypb"
)

const bufSize = 1024 * 1024

func newMockGrpcClient(t *testing.T, service *syncPeerService) proto.SyncPeerClient {
	t.Helper()

	lis := bufconn.Listen(bufSize)
	s := grpc.NewServer()
	proto.RegisterSyncPeerServer(s, service)

	go func() {
		if err := s.Serve(lis); err != nil {
			log.Fatal(err)
		}
	}()

	ctx := context.Background()
	conn, err := grpc.DialContext(ctx, "bufnet",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(
			func(ctx context.Context, address string) (net.Conn, error) {
				return lis.Dial()
			},
		),
	)

	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		defer conn.Close()
	})

	return proto.NewSyncPeerClient(conn)
}

func Test_syncPeerService_GetBlocks(t *testing.T) {
	t.Parallel()

	blocks := createMockBlocks(10)

	tests := []struct {
		name           string
		from           uint64
		latest         uint64
		blocks         []*types.Block
		receivedBlocks []*types.Block
		err            error
	}{
		{
			name:           "should send the blocks to the latest",
			from:           5,
			latest:         10,
			blocks:         blocks,
			receivedBlocks: blocks[4:], // from 5
			err:            io.EOF,
		},
		{
			name:           "should return ErrBlockNotFound",
			from:           5,
			latest:         10,
			blocks:         blocks[:8],
			receivedBlocks: blocks[4:8], // from 5
			err:            ErrBlockNotFound,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			blockMap := make(map[uint64]*types.Block)

			for _, b := range test.blocks {
				blockMap[b.Number()] = b
			}

			service := &syncPeerService{
				blockchain: &mockBlockchain{
					headerHandler: newSimpleHeaderHandler(test.latest),
					getBlockByNumberHandler: func(u uint64, _ bool) (*types.Block, bool) {
						block, ok := blockMap[u]
						if !ok {
							return nil, false
						}

						return block, true
					},
				},
			}

			client := newMockGrpcClient(t, service)

			stream, err := client.GetBlocks(context.Background(), &proto.GetBlocksRequest{
				From: test.from,
			})

			assert.NoError(t, err)

			count := 0

			for {
				protoBlock, err := stream.Recv()
				if err != nil {
					assert.Contains(t, err.Error(), test.err.Error())

					break
				}

				expected := test.receivedBlocks[count].MarshalRLP()

				assert.Equal(t, expected, protoBlock.Block)

				count++
			}
		})
	}
}

func TestGetStatus(t *testing.T) {
	t.Parallel()

	headerNumber := uint64(10)

	service := &syncPeerService{
		blockchain: &mockBlockchain{
			headerHandler: newSimpleHeaderHandler(headerNumber),
		},
	}

	client := newMockGrpcClient(t, service)

	status, err := client.GetStatus(context.Background(), &emptypb.Empty{})

	assert.NoError(t, err)
	assert.Equal(t, headerNumber, status.Number)
}
