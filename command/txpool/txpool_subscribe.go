package txpool

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/0xPolygon/polygon-edge/command/helper"
	txpoolProto "github.com/0xPolygon/polygon-edge/txpool/proto"
	"io"
	"os"
	"os/signal"
	"syscall"
)

// TxPoolSubscribeCommand is the command to monitor to the txpool events
type TxPoolSubscribeCommand struct {
	helper.Base
	Formatter *helper.FormatterFlag
	GRPC      *helper.GRPCFlag
}

// DefineFlags defines the command flags
func (t *TxPoolSubscribeCommand) DefineFlags() {
	t.Base.DefineFlags(t.Formatter, t.GRPC)

	t.FlagMap["added"] = helper.FlagDescriptor{
		Description: "Subscribes for added tx events to the TxPool",
		Arguments: []string{
			"LISTEN_ADDED",
		},
		ArgumentsOptional: true,
		FlagOptional:      true,
	}

	t.FlagMap["enqueued"] = helper.FlagDescriptor{
		Description: "Subscribes for enqueued tx events in the account queues",
		Arguments: []string{
			"LISTEN_ENQUEUED",
		},
		ArgumentsOptional: true,
		FlagOptional:      true,
	}

	t.FlagMap["promoted"] = helper.FlagDescriptor{
		Description: "Subscribes for promoted tx events in the TxPool",
		Arguments: []string{
			"LISTEN_PROMOTED",
		},
		ArgumentsOptional: true,
		FlagOptional:      true,
	}

	t.FlagMap["dropped"] = helper.FlagDescriptor{
		Description: "Subscribes for dropped tx events in the TxPool",
		Arguments: []string{
			"LISTEN_DROPPED",
		},
		ArgumentsOptional: true,
		FlagOptional:      true,
	}

	t.FlagMap["demoted"] = helper.FlagDescriptor{
		Description: "Subscribes for demoted tx events in the TxPool",
		Arguments: []string{
			"LISTEN_DEMOTED",
		},
		ArgumentsOptional: true,
		FlagOptional:      true,
	}

	t.FlagMap["pruned-promoted"] = helper.FlagDescriptor{
		Description: "Subscribes for pruned promoted tx events in the TxPool",
		Arguments: []string{
			"LISTEN_PRUNED_PROMOTED",
		},
		ArgumentsOptional: true,
		FlagOptional:      true,
	}

	t.FlagMap["pruned-enqueued"] = helper.FlagDescriptor{
		Description: "Subscribes for pruned enqueued tx events in the TxPool",
		Arguments: []string{
			"LISTEN_PRUNED_ENQUEUED",
		},
		ArgumentsOptional: true,
		FlagOptional:      true,
	}
}

// GetHelperText returns a simple description of the command
func (t *TxPoolSubscribeCommand) GetHelperText() string {
	return "Starts logging TxPool events for the specified types, or any if no types are specified"
}

func (t *TxPoolSubscribeCommand) GetBaseCommand() string {
	return "txpool subscribe"
}

// Help implements the cli.Command interface
func (t *TxPoolSubscribeCommand) Help() string {
	t.DefineFlags()

	return helper.GenerateHelp(t.Synopsis(), helper.GenerateUsage(t.GetBaseCommand(), t.FlagMap), t.FlagMap)
}

// Synopsis implements the cli.Command interface
func (t *TxPoolSubscribeCommand) Synopsis() string {
	return t.GetHelperText()
}

// Run implements the cli.Command interface
func (t *TxPoolSubscribeCommand) Run(args []string) int {
	flags := t.Base.NewFlagSet(t.GetBaseCommand(), t.Formatter, t.GRPC)

	var (
		added          bool
		promoted       bool
		enqueued       bool
		dropped        bool
		demoted        bool
		prunedPromoted bool
		prunedEnqueued bool
	)

	flags.BoolVar(&added, "added", false, "")
	flags.BoolVar(&promoted, "promoted", false, "")
	flags.BoolVar(&enqueued, "enqueued", false, "")
	flags.BoolVar(&dropped, "dropped", false, "")
	flags.BoolVar(&demoted, "demoted", false, "")
	flags.BoolVar(&prunedPromoted, "pruned-promoted", false, "")
	flags.BoolVar(&prunedEnqueued, "pruned-enqueued", false, "")

	if err := flags.Parse(args); err != nil {
		t.Formatter.OutputError(err)

		return 1
	}

	eventTypes := make([]txpoolProto.EventType, 0)
	if added {
		eventTypes = append(eventTypes, txpoolProto.EventType_ADDED)
	}

	if promoted {
		eventTypes = append(eventTypes, txpoolProto.EventType_PROMOTED)
	}

	if enqueued {
		eventTypes = append(eventTypes, txpoolProto.EventType_ENQUEUED)
	}

	if dropped {
		eventTypes = append(eventTypes, txpoolProto.EventType_DROPPED)
	}

	if demoted {
		eventTypes = append(eventTypes, txpoolProto.EventType_DEMOTED)
	}

	if prunedPromoted {
		eventTypes = append(eventTypes, txpoolProto.EventType_PRUNED_PROMOTED)
	}

	if prunedEnqueued {
		eventTypes = append(eventTypes, txpoolProto.EventType_PRUNED_ENQUEUED)
	}

	if len(eventTypes) == 0 {
		// Any kind of event subscription is default
		eventTypes = append(
			eventTypes,
			txpoolProto.EventType_ADDED,
			txpoolProto.EventType_PROMOTED,
			txpoolProto.EventType_ENQUEUED,
			txpoolProto.EventType_DROPPED,
			txpoolProto.EventType_DEMOTED,
			txpoolProto.EventType_PRUNED_PROMOTED,
			txpoolProto.EventType_PRUNED_ENQUEUED,
		)
	}

	conn, err := t.GRPC.Conn()
	if err != nil {
		t.Formatter.OutputError(err)

		return 1
	}

	clt := txpoolProto.NewTxnPoolOperatorClient(conn)
	ctx, cancelFn := context.WithCancel(context.Background())

	stream, err := clt.Subscribe(ctx, &txpoolProto.SubscribeRequest{
		Types: eventTypes,
	})
	if err != nil {
		t.Formatter.OutputError(err)
		cancelFn()

		return 1
	}

	doneCh := make(chan struct{})

	go func() {
		for {
			streamEvent, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				break
			}

			if err != nil {
				t.Formatter.OutputError(fmt.Errorf("failed to read event: %w", err))

				break
			}

			t.Formatter.OutputResult(&TxPoolEventResult{
				EventType: streamEvent.Type,
				TxHash:    streamEvent.TxHash,
			})
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

type TxPoolEventResult struct {
	EventType txpoolProto.EventType `json:"eventType"`
	TxHash    string                `json:"txHash"`
}

func (r *TxPoolEventResult) Output() string {
	var buffer bytes.Buffer

	// current number & hash
	buffer.WriteString("\n[TXPOOL EVENT]\n")
	buffer.WriteString(helper.FormatKV([]string{
		fmt.Sprintf("TYPE|%s", r.EventType),
		fmt.Sprintf("HASH|%s", r.TxHash),
	}))
	buffer.WriteString("\n")

	return buffer.String()
}
