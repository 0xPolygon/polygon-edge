package jsonrpc

import (
	"fmt"
	"net"

	"github.com/0xPolygon/minimal/api"
	"github.com/hashicorp/go-hclog"
	"github.com/valyala/fasthttp"
)

const (
	defaultHTTAddr  = "127.0.0.1"
	defaultPortAddr = 8545
)

// HTTPServer is an http server that serves jsonrpc requests
type HTTPServer struct {
	logger hclog.Logger
	d      *Dispatcher
	http   *fasthttp.Server
}

func startHTTPServer(d *Dispatcher, logger hclog.Logger, config ServerConfig) (Server, error) {
	tcpAddr, err := api.ReadAddrFromConfig(defaultHTTAddr, defaultPortAddr, config)
	if err != nil {
		return nil, err
	}
	lis, err := net.Listen("tcp", tcpAddr.String())
	if err != nil {
		return nil, err
	}

	httpLogger := logger.Named("JsonRPC-HTTP")
	httpLogger.Info("Running", "addr", tcpAddr.String())

	h := &HTTPServer{
		logger: httpLogger,
		d:      d,
	}
	h.serve(lis)
	return h, nil
}

func (h *HTTPServer) serve(lis net.Listener) {
	h.http = &fasthttp.Server{
		Handler: h.handler,
	}

	go func() {
		if err := h.http.Serve(lis); err != nil {
			h.logger.Info("Jsonrpc http closed: %v", err)
		}
	}()
}

func (h *HTTPServer) handler(ctx *fasthttp.RequestCtx) {
	if ctx.IsGet() {
		fmt.Fprintf(ctx, "Minimal JSON-RPC")
		return
	}
	if !ctx.IsPost() {
		fmt.Fprintf(ctx, "method %s not allowed", ctx.Method())
		return
	}
	resp, err := h.d.handle(serverHTTP, ctx.PostBody())
	if err != nil {
		fmt.Fprintf(ctx, err.Error())
		return
	}
	fmt.Fprintf(ctx, string(resp))
}

// Close implements the transport interface
func (h *HTTPServer) Close() error {
	return h.http.Shutdown()
}
