package consensus

import (
	"context"
	"log"

	"github.com/dogechain-lab/jury/blockchain"
	"github.com/dogechain-lab/jury/chain"
	"github.com/dogechain-lab/jury/helper/progress"
	"github.com/dogechain-lab/jury/network"
	"github.com/dogechain-lab/jury/secrets"
	"github.com/dogechain-lab/jury/state"
	"github.com/dogechain-lab/jury/txpool"
	"github.com/dogechain-lab/jury/types"
	"github.com/hashicorp/go-hclog"
	"google.golang.org/grpc"
)

// Consensus is the public interface for consensus mechanism
// Each consensus mechanism must implement this interface in order to be valid
type Consensus interface {
	// VerifyHeader verifies the header is correct
	VerifyHeader(parent, header *types.Header) error

	// GetBlockCreator retrieves the block creator (or signer) given the block header
	GetBlockCreator(header *types.Header) (types.Address, error)

	// PreStateCommit a hook to be called before finalizing state transition on inserting block
	PreStateCommit(header *types.Header, txn *state.Transition) error

	// GetSyncProgression retrieves the current sync progression, if any
	GetSyncProgression() *progress.Progression

	// Initialize initializes the consensus (e.g. setup data)
	Initialize() error

	// Start starts the consensus and servers
	Start() error

	// Close closes the connection
	Close() error
}

// Config is the configuration for the consensus
type Config struct {
	// Logger to be used by the backend
	Logger *log.Logger

	// Params are the params of the chain and the consensus
	Params *chain.Params

	// Config defines specific configuration parameters for the backend
	Config map[string]interface{}

	// Path is the directory path for the consensus protocol tos tore information
	Path string
}

type ConsensusParams struct {
	Context        context.Context
	Seal           bool
	Config         *Config
	Txpool         *txpool.TxPool
	Network        *network.Server
	Blockchain     *blockchain.Blockchain
	Executor       *state.Executor
	Grpc           *grpc.Server
	Logger         hclog.Logger
	Metrics        *Metrics
	SecretsManager secrets.SecretsManager
	BlockTime      uint64
}

// Factory is the factory function to create a discovery backend
type Factory func(
	*ConsensusParams,
) (Consensus, error)
