package sidecar

import (
	"context"
	"fmt"
	"time"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/server/proto"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/emptypb"
)

func GetCommand() *cobra.Command {
	sidecarCmd := &cobra.Command{
		Use:   "sidecar",
		Short: "Runs the sidecar middleware application",
		Run: func(cmd *cobra.Command, args []string) {
			outputter := command.InitializeOutputter(cmd)

			addr := helper.GetGRPCAddress(cmd)

			sidecar, err := newSidecar(addr)
			if err != nil {
				panic(err)
			}

			helper.HandleSignals(sidecar.Close, outputter)
		},
	}

	helper.RegisterGRPCAddressFlag(sidecarCmd)

	return sidecarCmd
}

type sidecar struct {
	clt     proto.SystemClient
	closeCh chan struct{}
}

func newSidecar(grpcAddress string) (*sidecar, error) {
	client, err := helper.GetSystemClientConnection(grpcAddress)
	if err != nil {
		return nil, err
	}

	s := &sidecar{
		clt:     client,
		closeCh: make(chan struct{}),
	}

	go s.run()

	return s, nil
}

func (s *sidecar) run() {
	lastBlock := int64(0)

	for {
		status, err := s.clt.GetStatus(context.Background(), &emptypb.Empty{})
		if err != nil {
			panic(err)
		}

		if lastBlock == status.Current.Number {
			continue
		}

		trace, err := s.clt.GetTrace(context.Background(), &proto.GetTraceRequest{Number: uint64(status.Current.Number)})
		if err != nil {
			panic(err)
		}

		fmt.Println("-----")
		fmt.Println(trace.AccountTrace)
		fmt.Println(trace.StorageTrace)

		select {
		case <-time.After(500 * time.Millisecond):
		case <-s.closeCh:
			return
		}
	}
}

func (s *sidecar) Close() {
	close(s.closeCh)
}
