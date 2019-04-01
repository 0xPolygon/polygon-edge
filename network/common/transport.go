package common

import (
	"crypto/ecdsa"
	"net"

	"github.com/umbracle/minimal/helper/enode"
	"github.com/umbracle/minimal/protocol"
)

type Instance struct {
	Protocol protocol.Protocol
	Handler  protocol.Handler
}

// Session is an open connection between two peers
type Session interface {
	// OpenStream opens a new stream within the session
	// OpenStream() (net.Conn, error)

	// NegociateProtocols negociates the sub-protocols
	NegociateProtocols(info *Info) ([]*Instance, error)

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
	Setup(priv *ecdsa.PrivateKey, backends []protocol.Backend, info *Info)

	// Connect connects with the remove connection
	Connect(net.Conn, enode.Enode) (Session, error)

	// Accept accepts the new connection
	Accept(net.Conn) (Session, error)
}
