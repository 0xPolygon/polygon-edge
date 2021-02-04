package command

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/0xPolygon/minimal/minimal/proto"
	"github.com/golang/protobuf/ptypes/empty"
)

// Watch is the command to watch to the blockchain events
type Watch struct {
	Meta
}

// Help implements the cli.Command interface
func (w *Watch) Help() string {
	return ""
}

// Synopsis implements the cli.Command interface
func (w *Watch) Synopsis() string {
	return ""
}

// Run implements the cli.Command interface
func (w *Watch) Run(args []string) int {
	flags := w.FlagSet("watch")
	if err := flags.Parse(args); err != nil {
		w.UI.Error(err.Error())
		return 1
	}

	conn, err := w.Conn()
	if err != nil {
		w.UI.Error(err.Error())
		return 1
	}

	clt := proto.NewSystemClient(conn)
	ctx, cancelFn := context.WithCancel(context.Background())

	stream, err := clt.Subscribe(ctx, &empty.Empty{})
	if err != nil {
		w.UI.Error(err.Error())
		cancelFn()
		return 1
	}

	doneCh := make(chan struct{})
	go func() {
		for {
			evnt, err := stream.Recv()
			if err != nil {
				w.UI.Error(fmt.Sprintf("failed to read event: %v", err))
				break
			}
			fmt.Println("-- evnt --")
			fmt.Println(evnt)
		}
		doneCh <- struct{}{}
	}()

	// wait for the user to quit with ctrl-c
	signalCh := make(chan os.Signal, 4)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)

	select {
	case <-signalCh:
	case <-doneCh:
	}
	cancelFn()

	return 0
}
