package backup

import (
	"context"
	"errors"
	"io"
	"os"

	"github.com/0xPolygon/polygon-sdk/helper/common"
	"github.com/0xPolygon/polygon-sdk/server/proto"
	"google.golang.org/grpc"
)

func fetchAndSaveBackup(conn *grpc.ClientConn, from, to uint64, outPath string) (*BackupResult, error) {
	// always create new file, throw error if the file exists
	fs, err := os.OpenFile(outPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return nil, err
	}

	clt := proto.NewSystemClient(conn)
	ctx, cancelFn := context.WithCancel(context.Background())
	defer cancelFn()

	stream, err := clt.Export(ctx, &proto.ExportRequest{
		From: from,
		To:   to,
	})
	if err != nil {
		return nil, err
	}

	res := <-processExportStream(stream, fs)

	fs.Close()
	if res.err != nil {
		os.Remove(outPath)
		return nil, err
	}

	return &BackupResult{From: *res.from, To: *res.to, Out: fs.Name()}, nil
}

type processExportStreamResult struct {
	from *uint64
	to   *uint64
	err  error
}

func processExportStream(stream proto.System_ExportClient, writer io.Writer) <-chan processExportStreamResult {
	resCh := make(chan processExportStreamResult, 1)
	signalCh := common.GetTerminationSignalCh()

	go func() {
		defer close(resCh)

		var from, to *uint64
		returnResult := func() {
			if from == nil || to == nil {
				resCh <- processExportStreamResult{nil, nil, errors.New("couldn't fetch any block")}
			} else {
				resCh <- processExportStreamResult{from, to, nil}
			}
		}

		for {
			evnt, err := stream.Recv()
			if err == io.EOF {
				returnResult()
				return
			}
			if err != nil {
				resCh <- processExportStreamResult{nil, nil, err}
				return
			}

			// write data
			if _, err := writer.Write(evnt.Data); err != nil {
				resCh <- processExportStreamResult{nil, nil, err}
			}

			// update result
			if from == nil {
				from = &evnt.From
			}
			to = &evnt.To

			// finish in the middle if received termination signal
			select {
			case <-signalCh:
				returnResult()
				return
			default:
			}
		}
	}()

	return resCh
}
