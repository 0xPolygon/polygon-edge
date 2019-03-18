package protocol

import (
	"context"
	"net"
)

// Handler is the handler of the msg for the protocol
type Handler interface {
	Init() error
	Close() error
}

// Protocol is a specification of an etheruem protocol
type Protocol struct {
	Name    string
	Version uint
	Length  uint64
}

// Backend is a protocol backend
type Backend interface {
	Protocol() Protocol
	Add(conn net.Conn, peerID string) error
}

// Factory is the factory method to create the protocol
type Factory func(ctx context.Context, m interface{}) (Backend, error)
