package monitor

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/0xPolygon/polygon-sdk/command/helper"
	"github.com/0xPolygon/polygon-sdk/server/proto"
	empty "google.golang.org/protobuf/types/known/emptypb"
)

// MonitorCommand is the command to Monitor to the blockchain events
type MonitorCommand struct {
	helper.Base
	Formatter *helper.FormatterFlag
	GRPC      *helper.GRPCFlag
}

// DefineFlags defines the command flags
func (m *MonitorCommand) DefineFlags() {
	m.Base.DefineFlags(m.Formatter, m.GRPC)
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
	m.DefineFlags()

	return helper.GenerateHelp(m.Synopsis(), helper.GenerateUsage(m.GetBaseCommand(), m.FlagMap), m.FlagMap)
}

// Synopsis implements the cli.Command interface
func (m *MonitorCommand) Synopsis() string {
	return m.GetHelperText()
}

// Run implements the cli.Command interface
func (m *MonitorCommand) Run(args []string) int {
	flags := m.Base.NewFlagSet(m.GetBaseCommand(), m.Formatter, m.GRPC)
	if err := flags.Parse(args); err != nil {
		m.Formatter.OutputError(err)
		return 1
	}

	conn, err := m.GRPC.Conn()
	if err != nil {
		m.Formatter.OutputError(err)
		return 1
	}

	clt := proto.NewSystemClient(conn)
	ctx, cancelFn := context.WithCancel(context.Background())

	stream, err := clt.Subscribe(ctx, &empty.Empty{})
	if err != nil {
		m.Formatter.OutputError(err)
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
				m.Formatter.OutputError(fmt.Errorf("failed to read event: %w", err))
				break
			}
			res := NewBlockEventResult(evnt)
			m.Formatter.OutputResult(res)
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

const (
	eventAdded   = "added"
	eventRemoved = "removed"
)

type BlockchainEvent struct {
	Type   string `json:"type"`
	Number int64  `json:"number"`
	Hash   string `json:"hash"`
}

type BlockChainEvents struct {
	Added   []BlockchainEvent `json:"added"`
	Removed []BlockchainEvent `json:"removed"`
}

type BlockEventResult struct {
	Events BlockChainEvents `json:"events"`
}

func NewBlockEventResult(e *proto.BlockchainEvent) *BlockEventResult {
	res := &BlockEventResult{
		Events: BlockChainEvents{
			Added:   make([]BlockchainEvent, len(e.Added)),
			Removed: make([]BlockchainEvent, len(e.Removed)),
		},
	}
	for i, add := range e.Added {
		res.Events.Added[i].Type = eventAdded
		res.Events.Added[i].Number = add.Number
		res.Events.Added[i].Hash = add.Hash
	}
	for i, rem := range e.Removed {
		res.Events.Removed[i].Type = eventRemoved
		res.Events.Removed[i].Number = rem.Number
		res.Events.Removed[i].Hash = rem.Hash
	}
	return res
}

func (r *BlockEventResult) Output() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[BLOCK EVENT]\n")
	for _, add := range r.Events.Added {
		buffer.WriteString(helper.FormatKV([]string{
			fmt.Sprintf("Event Type|%s", "ADD BLOCK"),
			fmt.Sprintf("Block Number|%d", add.Number),
			fmt.Sprintf("Block Hash|%s", add.Hash),
		}))
	}
	for _, rem := range r.Events.Removed {
		buffer.WriteString(helper.FormatKV([]string{
			fmt.Sprintf("Event Type|%s", "REMOVE BLOCK"),
			fmt.Sprintf("Block Number|%d", rem.Number),
			fmt.Sprintf("Block Hash|%s", rem.Hash),
		}))
	}

	return buffer.String()
}
