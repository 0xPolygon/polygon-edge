package jsonrpc

import "github.com/umbracle/minimal/helper/ipc"

func startIPC() {
	lis, err := ipc.Listen("minimal.ipc")
	if err != nil {
		panic(err)
	}

	h := HTTPServer{}
	if err := h.serve(lis); err != nil {
		panic(err)
	}
}
