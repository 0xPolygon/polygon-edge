package jsonrpc

import (
	"io/ioutil"
	"net"
	"net/http"

	"github.com/0xPolygon/minimal/api"
	"github.com/gorilla/websocket"
	"github.com/hashicorp/go-hclog"
)

var upgrader = websocket.Upgrader{}

const (
	defaultHTTAddr  = "127.0.0.1"
	defaultPortAddr = 8545
)

// HTTPServer is an http server that serves jsonrpc requests
type HTTPServer struct {
	logger hclog.Logger
	d      *Dispatcher
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
	go h.serve(lis)
	return h, nil
}

func (h *HTTPServer) handleWs(w http.ResponseWriter, req *http.Request) {
	c, err := upgrader.Upgrade(w, req, nil)
	if err != nil {
		return
	}
	defer c.Close()

	for {
		_, message, err := c.ReadMessage()
		if err != nil {
			break
		}
		go h.d.handleWs(message, c)
	}
}

func (h *HTTPServer) handle(w http.ResponseWriter, req *http.Request) {
	handleErr := func(err error) {
		w.Write([]byte(err.Error()))
		return
	}
	if req.Method == "GET" {
		w.Write([]byte("Minimal JSON-RPC"))
		return
	}
	if req.Method != "POST" {
		w.Write([]byte("method " + req.Method + " not allowed"))
		return
	}
	data, err := ioutil.ReadAll(req.Body)
	if err != nil {
		handleErr(err)
		return
	}
	resp, err := h.d.handle(serverHTTP, data)
	if err != nil {
		handleErr(err)
		return
	}
	w.Write(resp)
}

func (h *HTTPServer) serve(lis net.Listener) {
	mux := http.DefaultServeMux
	mux.HandleFunc("/", h.handle)
	mux.HandleFunc("/ws", h.handleWs)

	srv := http.Server{
		Handler: mux,
	}
	go func() {
		if err := srv.Serve(lis); err != nil {
			panic(err)
		}
	}()
}

// Close implements the transport interface
func (h *HTTPServer) Close() error {
	// return h.http.Shutdown()
	return nil
}
