package jsonrpc

import (
	"fmt"
	"net"

	"github.com/valyala/fasthttp"
)

// HTTPServer is an http server that serves jsonrpc requests
type HTTPServer struct {
	s    *Server
	http *fasthttp.Server
}

// StartHTTP starts an http connection
func StartHTTP(s *Server) {
	lis, err := net.Listen("tcp", "localhost:8080")
	if err != nil {
		panic(err)
	}

	h := HTTPServer{
		s: s,
	}
	if err := h.serve(lis); err != nil {
		panic(err)
	}
}

func (h *HTTPServer) serve(lis net.Listener) error {
	h.http = &fasthttp.Server{
		Handler: h.handler,
	}
	return h.http.Serve(lis)
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
	if _, err := h.s.handle(ctx.PostBody()); err != nil {
		fmt.Fprintf(ctx, err.Error())
	}
}

// Close closes the http server
func (h *HTTPServer) Close() error {
	return h.http.Shutdown()
}
