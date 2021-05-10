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

// MonitorCommand is the command to Monitor to the blockchain events
type MonitorCommand struct {
	Meta
}

// GetHelperText returns a simple description of the command
func (m *MonitorCommand) GetHelperText() string {
	return "Starts logging client activity"
}

// Help implements the cli.Command interface
func (m *MonitorCommand) Help() string {
	usage := "monitor"

	return m.GenerateHelp(m.Synopsis(), usage)
}

// Synopsis implements the cli.Command interface
func (m *MonitorCommand) Synopsis() string {
	return m.GetHelperText()
}

// Run implements the cli.Command interface
func (m *MonitorCommand) Run(args []string) int {
	flags := m.FlagSet("monitor")
	if err := flags.Parse(args); err != nil {
		m.UI.Error(err.Error())
		return 1
	}

	conn, err := m.Conn()
	if err != nil {
		m.UI.Error(err.Error())
		return 1
	}

	clt := proto.NewSystemClient(conn)
	ctx, cancelFn := context.WithCancel(context.Background())

	stream, err := clt.Subscribe(ctx, &empty.Empty{})
	if err != nil {
		m.UI.Error(err.Error())
		cancelFn()
		return 1
	}

	doneCh := make(chan struct{})
	go func() {
		for {
			evnt, err := stream.Recv()
			if err != nil {
				m.UI.Error(fmt.Sprintf("failed to read event: %v", err))
				break
			}
			fmt.Println("-- event --")
			for _, add := range evnt.Added {
				fmt.Printf("Add block: Num %d Hash %s\n", add.Number, add.Hash)
			}
			for _, del := range evnt.Removed {
				fmt.Printf("Delete block: Num %d Hash %s\n", del.Number, del.Hash)
			}
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
