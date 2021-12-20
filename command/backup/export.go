package backup

import (
	"context"
	"errors"
	"io"
	"os"

	"github.com/0xPolygon/polygon-sdk/helper/common"
	"github.com/0xPolygon/polygon-sdk/server/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func fetchAndSaveBackup(conn *grpc.ClientConn, from, to uint64, outPath string) (*BackupResult, error) {
	// always create new file, throw error if the file exists
	fs, err := os.OpenFile(outPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return nil, err
	}

	signalCh := common.GetTerminationSignalCh()
	ctx, cancelFn := context.WithCancel(context.Background())
	defer cancelFn()
	go func() {
		<-signalCh
		cancelFn()
	}()

	clt := proto.NewSystemClient(conn)
	stream, err := clt.Export(ctx, &proto.ExportRequest{
		From: from,
		To:   to,
	})
	if err != nil {
		return nil, err
	}

	resFrom, resTo, err := processExportStream(stream, fs)

	fs.Close()
	if err != nil {
		os.Remove(outPath)
		return nil, err
	}

	return &BackupResult{From: *resFrom, To: *resTo, Out: fs.Name()}, nil
}

func processExportStream(stream proto.System_ExportClient, writer io.Writer) (*uint64, *uint64, error) {
	var from, to *uint64
	getResult := func() (*uint64, *uint64, error) {
		if from == nil || to == nil {
			return nil, nil, errors.New("couldn't get any blocks")
		} else {
			return from, to, nil
		}
	}

	for {
		evnt, err := stream.Recv()
		if err == io.EOF || status.Code(err) == codes.Canceled {
			return getResult()
		}
		if err != nil {
			return nil, nil, err
		}

		// write data
		if _, err := writer.Write(evnt.Data); err != nil {
			return nil, nil, err
		}

		// update result
		if from == nil {
			from = &evnt.From
		}
		to = &evnt.To
	}
}
