package protocol2

import (
	"context"
	"fmt"

	v1 "github.com/0xPolygon/minimal/protocol2/proto"
	"google.golang.org/grpc"
)

// blockchain is the interface required by
// the syncer to connect to the blockchain
type blockchain interface {
}

// Syncer is a sync protocol
type Syncer struct {
}

func (s *Syncer) handleUser(conn *grpc.ClientConn) {
	clt := v1.NewV1Client(conn)
	fmt.Println(clt)

	// first, track the head
	clt.WatchBlocks(context.Background(), &v1.Empty{})
}

type headWatcher struct {
	ctx      context.Context
	cancelFn context.CancelFunc
}

func (h *headWatcher) start() {
	h.ctx, h.cancelFn = context.WithCancel(context.Background())
}

func (h *headWatcher) close() {
	h.cancelFn()
}

func (h *headWatcher) add(clt v1.V1Client) error {
	go h.track(clt)
	return nil
}

func (h *headWatcher) track(clt v1.V1Client) {
	recv, err := clt.WatchBlocks(h.ctx, &v1.Empty{})
	if err != nil {
		panic(err)
	}
	for {
		msg, err := recv.Recv()
		if err != nil {
			panic(err)
		}
		fmt.Println(msg)
	}
}
