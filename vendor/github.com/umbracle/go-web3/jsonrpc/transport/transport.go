package transport

import (
	"os"
	"strings"
)

// Transport is an inteface for transport methods to send jsonrpc requests
type Transport interface {
	// Call makes a jsonrpc request
	Call(method string, out interface{}, params ...interface{}) error

	// Close closes the transport connection if necessary
	Close() error
}

// PubSubTransport is a transport that allows subscriptions
type PubSubTransport interface {
	// Subscribe starts a subscription to a new event
	Subscribe(method string, callback func(b []byte)) (func() error, error)
}

const (
	wsPrefix = "ws://"
	wssPrefix = "wss://"
)

// NewTransport creates a new transport object
func NewTransport(url string) (Transport, error) {
	if strings.HasPrefix(url, wsPrefix) || strings.HasPrefix(url, wssPrefix) {
		return newWebsocket(url)
	}
	if _, err := os.Stat(url); !os.IsNotExist(err) {
		// path exists, it could be an ipc path
		return newIPC(url)
	}
	return newHTTP(url), nil
}
