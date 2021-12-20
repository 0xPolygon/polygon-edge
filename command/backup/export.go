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

	resCh, errCh := processExportStream(stream, fs)

	var res *BackupResult
	select {
	case res = <-resCh:
	case err = <-errCh:
	}

	fs.Close()
	if err != nil {
		os.Remove(outPath)
	}

	return res, err
}

func processExportStream(stream proto.System_ExportClient, fs *os.File) (<-chan *BackupResult, <-chan error) {
	resCh := make(chan *BackupResult, 1)
	errCh := make(chan error, 1)

	signalCh := common.GetTerminationSignalCh()

	go func() {
		defer close(resCh)
		defer close(errCh)

		var from, to *uint64
		returnResult := func() {
			if from == nil || to == nil {
				errCh <- errors.New("couldn't fetch any block")
			} else {
				resCh <- &BackupResult{
					From: *from,
					To:   *to,
					Out:  fs.Name(),
				}
			}
		}

		for {
			evnt, err := stream.Recv()
			if err == io.EOF {
				returnResult()
				return
			}
			if err != nil {
				errCh <- err
				return
			}

			// write data
			if _, err := fs.Write(evnt.Data); err != nil {
				errCh <- err
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

	return resCh, errCh
}
