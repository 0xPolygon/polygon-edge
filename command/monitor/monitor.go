package monitor

import (
	"context"
	"errors"
	"fmt"
	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/spf13/cobra"
	"io"

	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/server/proto"
	empty "google.golang.org/protobuf/types/known/emptypb"
)

func GetCommand() *cobra.Command {
	monitorCmd := &cobra.Command{
		Use:   "monitor",
		Short: "Starts logging block add / remove events on the blockchain",
		Run:   runCommand,
	}

	helper.RegisterGRPCAddressFlag(monitorCmd)

	return monitorCmd
}

func runCommand(cmd *cobra.Command, _ []string) {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	subscribeToEvents(
		outputter,
		helper.GetGRPCAddress(cmd),
	)
}

func subscribeToEvents(
	outputter command.OutputFormatter,
	grpcAddress string,
) {
	ctx, cancelFn := context.WithCancel(context.Background())
	defer cancelFn()

	stream, err := getMonitorStream(ctx, grpcAddress)
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

func getMonitorStream(
	ctx context.Context,
	grpcAddress string,
) (proto.System_SubscribeClient, error) {
	client, err := helper.GetSystemClientConnection(grpcAddress)
	if err != nil {
		return nil, err
	}

	return client.Subscribe(ctx, &empty.Empty{})
}

func runSubscribeLoop(
	stream proto.System_SubscribeClient,
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

			outputter.SetCommandResult(NewBlockEventResult(streamEvent))
			flushOutput()
		}

		doneCh <- struct{}{}
	}()

	select {
	case <-common.GetTerminationSignalCh():
	case <-doneCh:
	}
}
