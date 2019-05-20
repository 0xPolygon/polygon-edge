package protocol

import (
	"context"

	"github.com/hashicorp/go-hclog"
	"github.com/umbracle/minimal/network/common"
)

// Backend is a protocol backend
type Backend interface {
	Protocols() []*common.Protocol
	Run()
}

// Factory is the factory method to create the protocol
type Factory func(ctx context.Context, logger hclog.Logger, m interface{}, config map[string]interface{}) (Backend, error)
