package protocol2

import (
	"context"
	"fmt"

	v1 "github.com/0xPolygon/minimal/protocol2/proto"
)

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
		fmt.Println("- msg -")
		fmt.Println(msg)
	}
}
