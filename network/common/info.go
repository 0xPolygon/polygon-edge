package common

import (
	"net"

	"github.com/umbracle/minimal/helper/enode"
)

// Protocol is a wire protocol
type Protocol struct {
	Spec      ProtocolSpec
	HandlerFn func(conn net.Conn, peerID string) (ProtocolHandler, error)
}

// ProtocolHandler is the handler of the protocol
type ProtocolHandler interface {
	Info() (map[string]interface{}, error)
}

// ProtocolSpec is a specification of an etheruem protocol
type ProtocolSpec struct {
	Name    string
	Version uint
	Length  uint64
}

// Info is the information of a peer
type Info struct {
	Client       string
	Enode        *enode.Enode
	Capabilities Capabilities
	ListenPort   uint64
}

// Capability is a feature of the peer
type Capability struct {
	Protocol Protocol
}

// Capabilities is a list of capabilities of the peer
type Capabilities []*Capability
