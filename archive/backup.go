package archive

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/0xPolygon/polygon-sdk/helper/common"
	"github.com/0xPolygon/polygon-sdk/server/proto"
	"github.com/0xPolygon/polygon-sdk/types"
	"github.com/hashicorp/go-hclog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

func CreateBackup(conn *grpc.ClientConn, logger hclog.Logger, targetFrom uint64, targetTo *uint64, outPath string) (uint64, uint64, error) {
	// always create new file, throw error if the file exists
	fs, err := os.OpenFile(outPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return 0, 0, err
	}

	signalCh := common.GetTerminationSignalCh()
	ctx, cancelFn := context.WithCancel(context.Background())
	defer cancelFn()
	go func() {
		<-signalCh
		logger.Info("Caught termination signal, shutting down...")
		cancelFn()
	}()

	clt := proto.NewSystemClient(conn)

	to, toHash, err := determineTo(ctx, clt, targetTo)
	if err != nil {
		return 0, 0, err
	}

	stream, err := clt.Export(ctx, &proto.ExportRequest{
		From: targetFrom,
		To:   to,
	})
	if err != nil {
		return 0, 0, err
	}

	writeMetadata(fs, logger, to, toHash)
	resFrom, resTo, err := processExportStream(stream, logger, fs, targetFrom, to)

	fs.Close()
	if err != nil {
		os.Remove(outPath)
		return 0, 0, err
	}

	return *resFrom, *resTo, nil
}

func determineTo(ctx context.Context, clt proto.SystemClient, targetTo *uint64) (uint64, types.Hash, error) {
	status, err := clt.GetStatus(ctx, &emptypb.Empty{})
	if err != nil {
		return 0, types.Hash{}, err
	}

	if targetTo != nil && *targetTo < uint64(status.Current.Number) {
		// check the existence of the block when you have target to
		resp, err := clt.BlockByNumber(ctx, &proto.BlockByNumberRequest{Number: *targetTo})
		if err == nil {
			block := types.Block{}
			if err := block.UnmarshalRLP(resp.Data); err == nil {
				return block.Number(), block.Hash(), nil
			}
		}
	}
	// otherwise use latest block number as to
	return uint64(status.Current.Number), types.StringToHash(status.Current.Hash), nil
}

func writeMetadata(writer io.Writer, logger hclog.Logger, to uint64, toHash types.Hash) error {
	metadata := Metadata{
		Latest:     to,
		LatestHash: toHash,
	}
	_, err := writer.Write(metadata.MarshalRLP())
	logger.Info("Wrote metadata to backup", "latest", to, "hash", toHash)
	return err
}

func processExportStream(stream proto.System_ExportClient, logger hclog.Logger, writer io.Writer, targetFrom, targetTo uint64) (*uint64, *uint64, error) {
	var from, to *uint64

	getResult := func() (*uint64, *uint64, error) {
		if from == nil || to == nil {
			return nil, nil, errors.New("couldn't get any blocks")
		} else {
			return from, to, nil
		}
	}

	var total uint64
	showProgress := func(event *proto.ExportEvent) {
		num := event.To - event.From
		total += num
		expectedTo := targetTo
		if targetTo == 0 {
			expectedTo = event.Latest
		}
		expectedTotal := event.Latest - targetFrom
		progress := 100 * (float64(event.To) - float64(targetFrom)) / float64(expectedTotal)

		logger.Info(
			fmt.Sprintf("%d blocks are written", num),
			"total", total,
			"from", targetFrom,
			"to", expectedTo,
			"progress", fmt.Sprintf("%.2f%%", progress),
		)
	}

	for {
		event, err := stream.Recv()
		if errors.Is(io.EOF, err) || status.Code(err) == codes.Canceled {
			return getResult()
		}
		if err != nil {
			return nil, nil, err
		}

		if _, err := writer.Write(event.Data); err != nil {
			return nil, nil, err
		}

		if from == nil {
			from = &event.From
		}
		to = &event.To

		showProgress(event)
	}
}
