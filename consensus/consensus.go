package consensus

import (
	"context"
	"log"

	"github.com/0xPolygon/minimal/chain"
	"github.com/0xPolygon/minimal/types"
)

// Consensus is the interface for consensus
type Consensus interface {
	// VerifyHeader verifies the header is correct
	VerifyHeader(parent *types.Header, header *types.Header, uncle, seal bool) error

	// Seal seals the block
	Seal(ctx context.Context, block *types.Block) (*types.Block, error)

	// Close closes the connection
	Close() error
}

// Config is the configuration for the consensus
type Config struct {
	// Logger to be used by the backend
	Logger *log.Logger

	// Params are the params of the chain and the consensus
	Params *chain.Params

	// Specific configuration parameters for the backend
	Config map[string]interface{}
}

// Factory is the factory function to create a discovery backend
type Factory func(context.Context, *Config) (Consensus, error)
