package network

import (
	"crypto/ecdsa"
	"net"
	"time"
)

// Stream is a stream inside a session
type Stream interface {
	net.Conn
	Protocol() ProtocolSpec
}

// Session is an open connection between two peers
type Session interface {
	// Stream returns the set of streams inside the session
	Streams() []Stream

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
