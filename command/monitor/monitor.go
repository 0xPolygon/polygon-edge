package monitor

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/0xPolygon/minimal/command/helper"
	"github.com/0xPolygon/minimal/minimal/proto"
	empty "google.golang.org/protobuf/types/known/emptypb"
)

// MonitorCommand is the command to Monitor to the blockchain events
type MonitorCommand struct {
	helper.Meta
}

// GetHelperText returns a simple description of the command
func (m *MonitorCommand) GetHelperText() string {
	return "Starts logging block add / remove events on the blockchain"
}

func (m *MonitorCommand) GetBaseCommand() string {
	return "monitor"
}

// Help implements the cli.Command interface
func (m *MonitorCommand) Help() string {
	m.Meta.DefineFlags()

	return helper.GenerateHelp(m.Synopsis(), helper.GenerateUsage(m.GetBaseCommand(), m.FlagMap), m.FlagMap)
}

// Synopsis implements the cli.Command interface
func (m *MonitorCommand) Synopsis() string {
	return m.GetHelperText()
}

// Run implements the cli.Command interface
func (m *MonitorCommand) Run(args []string) int {
	flags := m.FlagSet(m.GetBaseCommand())
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
			if err == io.EOF {
				break
			}
			if err != nil {
				m.UI.Error(fmt.Sprintf("Failed to read event: %v", err))
				break
			}

			m.UI.Info("\n[BLOCK EVENT]\n")
			for _, add := range evnt.Added {
				m.UI.Info(helper.FormatKV([]string{
					fmt.Sprintf("Event Type|%s", "ADD BLOCK"),
					fmt.Sprintf("Block Number|%d", add.Number),
					fmt.Sprintf("Block Hash|%s", add.Hash),
				}))
			}
			for _, del := range evnt.Removed {
				m.UI.Info(helper.FormatKV([]string{
					fmt.Sprintf("Event Type|%s", "REMOVE BLOCK"),
					fmt.Sprintf("Block Number|%d", del.Number),
					fmt.Sprintf("Block Hash|%s", del.Hash),
				}))
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
