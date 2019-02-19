package consensus

import (
	"context"
	"log"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/umbracle/minimal/chain"
	"github.com/umbracle/minimal/state"
)

// Consensus is the interface for consensus
type Consensus interface {
	// VerifyHeader verifies the header is correct
	VerifyHeader(parent *types.Header, header *types.Header, uncle, seal bool) error

	// Author checks the author of the header
	Author(header *types.Header) (common.Address, error)

	// Seal seals the block
	Seal(ctx context.Context, block *types.Block) (*types.Block, error)

	// Prepare runs before processing the head during mining.
	Prepare(parent *types.Header, header *types.Header) error

	// Finalize runs after the block has been processed
	Finalize(txn *state.Txn, block *types.Block) error

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

// TODO, remove close method and use the context
// TODO, move prepare to seal
