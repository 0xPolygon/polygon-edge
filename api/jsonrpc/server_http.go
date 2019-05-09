package jsonrpc

import (
	"fmt"
	"net"
	"reflect"
	"strconv"

	"github.com/hashicorp/go-hclog"
	"github.com/valyala/fasthttp"
)

// HTTPServer is an http server that serves jsonrpc requests
type HTTPServer struct {
	logger hclog.Logger
	d      *Dispatcher
	http   *fasthttp.Server
}

func startHTTPServer(d *Dispatcher, logger hclog.Logger, config ServerConfig) (Server, error) {
	addr := "127.0.0.1"
	port := 8080

	addrRaw, ok := config["addr"]
	if ok {
		addr, ok = addrRaw.(string)
		if !ok {
			return nil, fmt.Errorf("could not convert addr '%s' to string", addrRaw)
		}
	}

	var err error
	portRaw, ok := config["port"]
	if ok {
		switch obj := portRaw.(type) {
		case string:
			port, err = strconv.Atoi(obj)
			if err != nil {
				return nil, fmt.Errorf("could not convert port '%s' to int", portRaw)
			}
		case int:
			port = obj
		case float64:
			port = int(obj)
		default:
			return nil, fmt.Errorf("could not parse port from '%s' of type %s", portRaw, reflect.TypeOf(portRaw).String())
		}
	}

	ipAddr := net.ParseIP(addr)
	if ipAddr == nil {
		return nil, fmt.Errorf("could not parse addr '%s'", addr)
	}

	tcpAddr := net.TCPAddr{
		IP:   ipAddr,
		Port: port,
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
	if _, err := h.d.handle(serverHTTP, ctx.PostBody()); err != nil {
		fmt.Fprintf(ctx, err.Error())
	}
}

// Close implements the transport interface
func (h *HTTPServer) Close() error {
	return h.http.Shutdown()
}
