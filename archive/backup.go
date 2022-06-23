package archive

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/server/proto"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// CreateBackup fetches blockchain data with the specific range via gRPC
// and save this data as binary archive to given path
func CreateBackup(
	conn *grpc.ClientConn,
	logger hclog.Logger,
	from uint64,
	to *uint64,
	outPath string,
) (uint64, uint64, error) {
	// always create new file, throw error if the file exists
	fs, err := os.OpenFile(outPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return 0, 0, err
	}

	closeFile := func() error {
		if err := fs.Close(); err != nil {
			logger.Error("an error occurred while closing file", "err", err)

			return err
		}

		return nil
	}
	removeFile := func() {
		if err := os.Remove(outPath); err != nil {
			logger.Error("an error occurred while removing file", "err", err)
		}
	}
	// clean up function for the file when error occurs in the middle of function
	closeAndRemoveFile := func() {
		if err := closeFile(); err == nil {
			removeFile()
		}
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

	reqTo, reqToHash, err := determineTo(ctx, clt, to)
	if err != nil {
		closeAndRemoveFile()

		return 0, 0, err
	}

	stream, err := clt.Export(ctx, &proto.ExportRequest{
		From: from,
		To:   reqTo,
	})
	if err != nil {
		closeAndRemoveFile()

		return 0, 0, err
	}

	if err := writeMetadata(fs, logger, reqTo, reqToHash); err != nil {
		closeAndRemoveFile()

		return 0, 0, err
	}

	resFrom, resTo, err := processExportStream(stream, logger, fs, from, reqTo)
	if err != nil {
		closeAndRemoveFile()

		return 0, 0, err
	}

	if err := closeFile(); err != nil {
		removeFile()

		return 0, 0, err
	}

	return *resFrom, *resTo, nil
}

func determineTo(ctx context.Context, clt proto.SystemClient, to *uint64) (uint64, types.Hash, error) {
	status, err := clt.GetStatus(ctx, &emptypb.Empty{})
	if err != nil {
		return 0, types.Hash{}, err
	}

	if to != nil && *to < uint64(status.Current.Number) {
		// check the existence of the block when you have targetTo
		resp, err := clt.BlockByNumber(ctx, &proto.BlockByNumberRequest{Number: *to})
		if err == nil && resp != nil {
			block := types.Block{}
			if err := block.UnmarshalRLP(resp.Data); err == nil {
				// can use targetTo only if the node has the block at the specific height
				return block.Number(), block.Hash(), nil
			}
		}
	}

	// otherwise use latest block number as to
	return uint64(status.Current.Number), types.StringToHash(status.Current.Hash), nil
}

// writeMetadata writes the latest block height and the block hash to the writer
func writeMetadata(writer io.Writer, logger hclog.Logger, to uint64, toHash types.Hash) error {
	metadata := Metadata{
		Latest:     to,
		LatestHash: toHash,
	}

	_, err := writer.Write(metadata.MarshalRLP())
	if err != nil {
		return err
	}

	logger.Info("Wrote metadata to backup", "latest", to, "hash", toHash)

	return err
}

func processExportStream(
	stream proto.System_ExportClient,
	logger hclog.Logger,
	writer io.Writer,
	targetFrom, targetTo uint64,
) (*uint64, *uint64, error) {
	var from, to *uint64

	getResult := func() (*uint64, *uint64, error) {
		if from == nil || to == nil {
			return nil, nil, errors.New("couldn't get any blocks")
		}

		return from, to, nil
	}

	var total uint64

	showProgress := func(event *proto.ExportEvent) {
		num := event.To - event.From
		total += num
		expectedTo := targetTo

		if targetTo == 0 {
			expectedTo = event.Latest
		}

		expectedTotal := targetTo - targetFrom
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
