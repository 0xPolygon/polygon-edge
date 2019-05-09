package jsonrpc

import "github.com/hashicorp/go-hclog"

type serverType int

const (
	serverIPC serverType = iota
	serverHTTP
	serverWS
)

var defaultServers = map[serverType]ServerFactory{
	serverHTTP: startHTTPServer,
	serverIPC:  startIPCServer,
}

// ServerConfig is the configuration for each server
type ServerConfig map[string]interface{}

// ServerFactory is a factory method to create servers
type ServerFactory func(d *Dispatcher, logger hclog.Logger, config ServerConfig) (Server, error)

// Server is a communication interface for the server
type Server interface {
	// Close shutdowns the server and closes any open connection
	Close() error
}
