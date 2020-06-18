package jsonrpc

import (
	"errors"

	"github.com/0xPolygon/minimal/helper/ipc"
	"github.com/hashicorp/go-hclog"
)

const defaultIPCPath = "/tmp/minimal.ipc"

func startIPCServer(d *Dispatcher, logger hclog.Logger, config ServerConfig) (Server, error) {
	path := defaultIPCPath

	pathRaw, ok := config["path"]
	if ok {
		path, ok = pathRaw.(string)
		if !ok {
			return nil, errors.New("could not convert path to a string")
		}
	}

	lis, err := ipc.Listen(path)
	if err != nil {
		return nil, err
	}

	ipcLogger := logger.Named("JsonRPC-IPC")
	ipcLogger.Info("Running", "path", path)

	h := &HTTPServer{
		logger: ipcLogger,
		d:      d,
	}
	h.serve(lis)
	return h, nil
}
