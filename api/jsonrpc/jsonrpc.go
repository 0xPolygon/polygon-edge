package jsonrpc

import (
	"fmt"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-multierror"
	"github.com/umbracle/minimal/api"
	"github.com/umbracle/minimal/minimal"
)

type serverType int

// NOTE: Since ws runs on top of the http connection, serverWS will likely be removed

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
	default:
		panic("TODO")
	}
}

// JSONRPC is an API backend
type JSONRPC struct {
	servers map[serverType]Server
}

// Close implements the API interface
func (j *JSONRPC) Close() error {
	var errs error
	for _, i := range j.servers {
		if err := i.Close(); err != nil {
			errs = multierror.Append(errs, err)
		}
	}
	return errs
}

// Factory is the factory method for the api backend
func Factory(logger hclog.Logger, m interface{}, config map[string]interface{}) (api.API, error) {
	handler := &JSONRPC{
		servers: map[serverType]Server{},
	}

	dispatcher := newDispatcher()
	dispatcher.minimal = m.(*minimal.Minimal)

	// start all the servers unless explicetly specified
	for typ, f := range defaultServers {
		conf, endpoints, disabled, err := getServerConfig(config, typ.String())
		if err != nil {
			return nil, err
		}

		if !disabled {
			server, err := f(dispatcher, logger, conf)
			if err != nil {
				return nil, err
			}
			handler.servers[typ] = server

			// Enable the endpoints for the server type
			dispatcher.enableEndpoints(typ, endpoints)
		}
	}

	return handler, nil
}

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

var defaultEndpoints = []string{
	"eth",
	"web3",
	"net",
}

func getServerConfig(config map[string]interface{}, field string) (map[string]interface{}, []string, bool, error) {
	// Enabled api endpoints by default
	endpoints := defaultEndpoints

	confRaw, ok := config[field]
	if !ok {
		// empty configuration
		return nil, endpoints, false, nil
	}

	conf, ok := confRaw.(map[string]interface{})
	if !ok {
		// the configuration is not a map
		return nil, nil, true, fmt.Errorf("Bad configuration for server %s", field)
	}

	if disabledRaw, ok := conf["disabled"]; ok {
		if disabled, ok := disabledRaw.(bool); ok && disabled {
			// there is a disabled flag, dont run this server
			return nil, nil, true, nil
		}
	}

	endpointsRaw, ok := conf["endpoints"]
	if ok {
		endpoints, ok = endpointsRaw.([]string)
		if !ok {
			return nil, nil, true, fmt.Errorf("could not get the enabled endpoints")
		}
		delete(conf, "endpoints")
	}
	return conf, endpoints, false, nil
}
