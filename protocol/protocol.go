package protocol

import (
	"context"
	"net"
)

// Protocol is a specification of an etheruem protocol
type Protocol struct {
	Name    string
	Version uint
	Length  uint64
}

// Handler is a backend reference of the peer
type Handler interface {
	Info() (map[string]interface{}, error)
}

// Backend is a protocol backend
type Backend interface {
	Protocol() Protocol
	Add(conn net.Conn, peerID string) (Handler, error)
}

// Factory is the factory method to create the protocol
type Factory func(ctx context.Context, m interface{}, config map[string]interface{}) (Backend, error)
