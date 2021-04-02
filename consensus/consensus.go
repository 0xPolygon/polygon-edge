package consensus

import (
	"context"
	"crypto/ecdsa"
	"log"

	"github.com/0xPolygon/minimal/blockchain"
	"github.com/0xPolygon/minimal/chain"
	"github.com/0xPolygon/minimal/state"
	"github.com/0xPolygon/minimal/txpool"
	"github.com/0xPolygon/minimal/types"
	"github.com/hashicorp/go-hclog"
)

// Consensus is the interface for consensus
type Consensus interface {
	// VerifyHeader verifies the header is correct
	VerifyHeader(parent, header *types.Header) error

	// TODO REMOVE
	Prepare(header *types.Header) error

	// TODO REMOVE
	Seal(block *types.Block, ctx context.Context) (*types.Block, error)

	StartSeal()

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

	// Path for the consensus protocol tos tore information
	Path string
}

// Factory is the factory function to create a discovery backend
type Factory func(context.Context, *Config, *txpool.TxPool, *blockchain.Blockchain, *state.Executor, *ecdsa.PrivateKey, hclog.Logger) (Consensus, error)

// Istanbul is a consensus engine to avoid byzantine failure
type Istanbul interface {
	Consensus

	// Start starts the engine
	Start(currentBlock func(bool) *types.Block, hasBadBlock func(hash types.Hash) bool) error

	// Stop stops the engine
	Stop() error
}
