package subscribe

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/helper/common"
	txpoolProto "github.com/0xPolygon/polygon-edge/txpool/proto"
	"github.com/spf13/cobra"
)

func GetCommand() *cobra.Command {
	txPoolSubscribeCmd := &cobra.Command{
		Use:   "subscribe",
		Short: "Logs specific TxPool events",
		Run:   runCommand,
	}

	setFlags(txPoolSubscribeCmd)

	return txPoolSubscribeCmd
}

func setFlags(cmd *cobra.Command) {
	params.initEventMap()

	cmd.Flags().BoolVar(
		params.eventSubscriptionMap[txpoolProto.EventType_ADDED],
		addedFlag,
		false,
		"should subscribe to added events",
	)
	cmd.Flags().BoolVar(
		params.eventSubscriptionMap[txpoolProto.EventType_PROMOTED],
		promotedFlag,
		false,
		"should subscribe to promoted events",
	)
	cmd.Flags().BoolVar(
		params.eventSubscriptionMap[txpoolProto.EventType_ENQUEUED],
		enqueuedFlag,
		false,
		"should subscribe to enqueued events",
	)
	cmd.Flags().BoolVar(
		params.eventSubscriptionMap[txpoolProto.EventType_DROPPED],
		droppedFlag,
		false,
		"should subscribe to dropped events",
	)
	cmd.Flags().BoolVar(
		params.eventSubscriptionMap[txpoolProto.EventType_DEMOTED],
		demotedFlag,
		false,
		"should subscribe to demoted events",
	)
	cmd.Flags().BoolVar(
		params.eventSubscriptionMap[txpoolProto.EventType_PRUNED_PROMOTED],
		prunedPromotedFlag,
		false,
		"should subscribe to pruned promoted tx events in the TxPool",
	)
	cmd.Flags().BoolVar(
		params.eventSubscriptionMap[txpoolProto.EventType_PRUNED_ENQUEUED],
		prunedEnqueuedFlag,
		false,
		"should subscribe to pruned enqueued tx events in the TxPool",
	)
}

func runCommand(cmd *cobra.Command, _ []string) {
	outputter := command.InitializeOutputter(cmd)

	params.init()

	subscribeToEvents(
		outputter,
		&txpoolProto.SubscribeRequest{
			Types: params.supportedEvents,
		},
		helper.GetGRPCAddress(cmd),
	)
}

func subscribeToEvents(
	outputter command.OutputFormatter,
	subscribeRequest *txpoolProto.SubscribeRequest,
	grpcAddress string,
) {
	ctx, cancelFn := context.WithCancel(context.Background())
	defer cancelFn()

	stream, err := getSubscribeStream(ctx, grpcAddress, subscribeRequest)
	if err != nil {
		outputter.SetError(err)
		outputter.WriteOutput()

		return
	}

	runSubscribeLoop(
		stream,
		outputter,
	)
}

func getSubscribeStream(
	ctx context.Context,
	grpcAddress string,
	subscribeRequest *txpoolProto.SubscribeRequest,
) (txpoolProto.TxnPoolOperator_SubscribeClient, error) {
	client, err := helper.GetTxPoolClientConnection(
		grpcAddress,
	)
	if err != nil {
		return nil, err
	}

	return client.Subscribe(
		ctx,
		subscribeRequest,
	)
}

func runSubscribeLoop(
	stream txpoolProto.TxnPoolOperator_SubscribeClient,
	outputter command.OutputFormatter,
) {
	doneCh := make(chan struct{})

	flushOutput := func() {
		outputter.SetError(nil)
		outputter.WriteOutput()
	}

	go func() {
		defer close(doneCh)

		for {
			streamEvent, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				break
			}

			if err != nil {
				outputter.SetError(fmt.Errorf("failed to read event: %w", err))
				outputter.WriteOutput()

				break
			}

			outputter.SetCommandResult(&TxPoolEventResult{
				EventType: streamEvent.Type,
				TxHash:    streamEvent.TxHash,
			})
			flushOutput()
		}

		doneCh <- struct{}{}
	}()

	select {
	case <-common.GetTerminationSignalCh():
	case <-doneCh:
	}
}
