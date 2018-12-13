package jsonrpc

import (
	"bytes"
	"io"
	"net/http"
	"net/rpc"
)

// rpcRequest represents a RPC request.
// rpcRequest implements the io.ReadWriteCloser interface.
type rpcRequest struct {
	rpc  *rpc.Server
	r    io.Reader     // holds the JSON formated RPC request
	rw   io.ReadWriter // holds the JSON formated RPC response
	done chan bool     // signals then end of the RPC request
}

// NewRPCRequest returns a new rpcRequest.
func NewRPCRequest(rpc *rpc.Server, r io.Reader) *rpcRequest {
	var buf bytes.Buffer
	done := make(chan bool)
	return &rpcRequest{rpc, r, &buf, done}
}

// Read implements the io.ReadWriteCloser Read method.
func (r *rpcRequest) Read(p []byte) (n int, err error) {
	return r.r.Read(p)
}

// Write implements the io.ReadWriteCloser Write method.
func (r *rpcRequest) Write(p []byte) (n int, err error) {
	return r.rw.Write(p)
}

// Close implements the io.ReadWriteCloser Close method.
func (r *rpcRequest) Close() error {
	r.done <- true
	return nil
}

// Call invokes the RPC request, waits for it to complete, and returns the results.
func (r *rpcRequest) Call() io.Reader {
	go ServeConn(r.rpc, r)
	<-r.done
	return r.rw
}

// ServeConn runs the JSON-RPC server on a single connection.
// ServeConn blocks, serving the connection until the client hangs up.
// The caller typically invokes ServeConn in a go statement.
func ServeConn(rpc *rpc.Server, conn io.ReadWriteCloser) {
	rpc.ServeCodec(NewEthCodec(conn))
}

func ServeHttp(rpc *rpc.Server) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		defer req.Body.Close()
		w.Header().Set("Content-Type", "application/json")
		res := NewRPCRequest(rpc, req.Body).Call()
		io.Copy(w, res)
	})
}
