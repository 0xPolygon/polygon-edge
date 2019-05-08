package jsonrpc

import (
	"errors"

	"github.com/hashicorp/go-hclog"
	"github.com/umbracle/minimal/helper/ipc"
)

const defaultIPCPath = "/tmp/minimal.ipc"

func startIPCTransport(server *Server, logger hclog.Logger, config TransportConfig) (Transport, error) {
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
		s:      server,
	}
	h.serve(lis)
	return h, nil
}
