package jsonrpc

import (
	"fmt"
	"io/ioutil"
	"net"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/hashicorp/go-hclog"
)

var upgrader = websocket.Upgrader{}

var (
	defaultHttpAddr = &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8545}
)

type serverType int

const (
	serverIPC serverType = iota
	serverHTTP
	serverWS
)

func (s serverType) String() string {
	switch s {
	case serverIPC:
		return "ipc"
	case serverHTTP:
		return "http"
	case serverWS:
		return "ws"
	default:
		panic("BUG: Not expected")
	}
}

// JSONRPC is an API backend
type JSONRPC struct {
	logger     hclog.Logger
	config     *Config
	dispatcher dispatcherImpl
}

type dispatcherImpl interface {
	HandleWs(reqBody []byte, conn wsConn) ([]byte, error)
	Handle([]byte) ([]byte, error)
}

type Config struct {
	Store blockchainInterface
	Addr  *net.TCPAddr
}

// NewJSONRPC returns the JsonRPC http server
func NewJSONRPC(logger hclog.Logger, config *Config) (*JSONRPC, error) {
	if config.Addr == nil {
		config.Addr = defaultHttpAddr
	}
	srv := &JSONRPC{
		logger:     logger.Named("jsonrpc"),
		config:     config,
		dispatcher: newDispatcher(logger, config.Store),
	}

	// start http server
	if err := srv.setupHTTP(); err != nil {
		return nil, err
	}
	return srv, nil
}

func (j *JSONRPC) setupHTTP() error {
	j.logger.Info("http server started", "addr", j.config.Addr.String())

	lis, err := net.Listen("tcp", j.config.Addr.String())
	if err != nil {
		return err
	}

	mux := http.DefaultServeMux
	mux.HandleFunc("/", j.handle)
	mux.HandleFunc("/ws", j.handleWs)

	srv := http.Server{
		Handler: mux,
	}
	go func() {
		if err := srv.Serve(lis); err != nil {
			j.logger.Error("closed http connection", "err", err)
		}
	}()
	return nil
}

type wrapWsConn struct {
	conn *websocket.Conn
}

func (w *wrapWsConn) WriteMessage(b []byte) error {
	return w.conn.WriteMessage(0, b)
}

func (j *JSONRPC) handleWs(w http.ResponseWriter, req *http.Request) {
	c, err := upgrader.Upgrade(w, req, nil)
	if err != nil {
		return
	}
	defer c.Close()

	wrapConn := &wrapWsConn{conn: c}
	for {
		_, message, err := c.ReadMessage()
		if err != nil {
			break
		}
		go func() {
			resp, err := j.dispatcher.HandleWs(message, wrapConn)
			if err != nil {
				wrapConn.WriteMessage(resp)
			} else {
				wrapConn.WriteMessage([]byte(fmt.Sprintf(err.Error())))
			}
		}()
	}
}

func (j *JSONRPC) handle(w http.ResponseWriter, req *http.Request) {
	handleErr := func(err error) {
		w.Write([]byte(err.Error()))
		return
	}
	if req.Method == "GET" {
		w.Write([]byte("PolygonSDK JSON-RPC"))
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
	resp, err := j.dispatcher.Handle(data)
	if err != nil {
		handleErr(err)
		return
	}
	w.Write(resp)
}
