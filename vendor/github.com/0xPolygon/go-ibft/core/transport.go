package core

import "github.com/0xPolygon/go-ibft/messages/proto"

// Transport defines an interface
// the node uses to communicate with other peers
type Transport interface {
	// Multicast multicasts the message to other peers
	Multicast(message *proto.Message)
}
