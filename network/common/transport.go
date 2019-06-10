package common

import (
	"crypto/ecdsa"
	"time"
)

type Instance struct {
	Protocol *Protocol
	Handler  ProtocolHandler
}

// Session is an open connection between two peers
type Session interface {
	// Protocols returns the set of protocols the session uses
	Protocols() []*Instance

	// Info returns the information of the network
	GetInfo() Info

	// IsClosed returns if the session has been closed
	IsClosed() bool

	// Close closes the connection
	Close() error
}

// Transport is a generic network transport protocol
type Transport interface {
	// Setup starts the protocol with the given private key
	Setup(priv *ecdsa.PrivateKey, backends []*Protocol, info *Info, config map[string]interface{}) error

	// DialTimeout connects to the address within a given timeout.
	DialTimeout(addr string, timeout time.Duration) (Session, error)

	// Accept accepts the new session
	Accept() (Session, error)

	// Close closes the transport
	Close() error
}
