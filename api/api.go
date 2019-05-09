package api

import (
	"fmt"
	"net"
	"reflect"
	"strconv"

	"github.com/hashicorp/go-hclog"
)

// An API backend exposes data to external interfaces
type API interface {
	Close() error
}

// Factory is the factory function to create an api backend
type Factory func(logger hclog.Logger, minimal interface{}, config map[string]interface{}) (API, error)

// ReadAddrFromConfig reads a tcp address from a config map
func ReadAddrFromConfig(defaultAddr string, defaultPort int, config map[string]interface{}) (*net.TCPAddr, error) {
	addr := defaultAddr
	port := defaultPort

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

	tcpAddr := &net.TCPAddr{
		IP:   ipAddr,
		Port: port,
	}
	return tcpAddr, nil
}
