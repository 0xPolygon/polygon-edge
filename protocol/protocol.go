package protocol

import (
	"context"

	"github.com/0xPolygon/minimal/network"
	"github.com/hashicorp/go-hclog"
)

// Backend is a protocol backend
type Backend interface {
	Protocols() []*network.Protocol
	Run()
}

// Factory is the factory method to create the protocol
type Factory func(ctx context.Context, logger hclog.Logger, m interface{}, config map[string]interface{}) (Backend, error)
