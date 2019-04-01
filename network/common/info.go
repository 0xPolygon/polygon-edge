package common

import (
	"github.com/umbracle/minimal/helper/enode"
	"github.com/umbracle/minimal/protocol"
)

// Info is the information of a peer
type Info struct {
	Client       string
	Enode        *enode.Enode
	Capabilities Capabilities
	ListenPort   uint64
}

// Capability is a feature of the peer
type Capability struct {
	Protocol protocol.Protocol
	Backend  protocol.Backend
}

// Capabilities is a list of capabilities of the peer
type Capabilities []*Capability
