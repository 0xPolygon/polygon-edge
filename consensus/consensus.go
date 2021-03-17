package consensus

import (
	"context"
	"crypto/ecdsa"
	"log"

	"github.com/0xPolygon/minimal/blockchain/storage"
	"github.com/0xPolygon/minimal/chain"
	"github.com/0xPolygon/minimal/types"
	"github.com/hashicorp/go-hclog"
)

// Consensus is the interface for consensus
type Consensus interface {
	// VerifyHeader verifies the header is correct
	VerifyHeader(parent, header *types.Header, uncle, seal bool) error

	//Prepare initializes the consensus fields of a block header according to the
	//rules of a particular engine. The changes are executed inline.
	Prepare(header *types.Header) error

	// Seal seals the block
	Seal(block *types.Block, ctx context.Context) (*types.Block, error)

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
type Factory func(context.Context, *Config, *ecdsa.PrivateKey, storage.Storage, hclog.Logger) (Consensus, error)
