package backend

import (
	"context"
	"log"

	"github.com/0xPolygon/polygon-edge/blockchain"
	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/helper/progress"
	"github.com/0xPolygon/polygon-edge/network"
	"github.com/0xPolygon/polygon-edge/secrets"
	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/txpool"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"google.golang.org/grpc"
)

// Backend is the public interface for backend mechanism
// Each backend mechanism must implement this interface in order to be valid
type Backend interface {
	// VerifyHeader verifies the header is correct
	VerifyHeader(header *types.Header) error

	// ProcessHeaders updates the snapshot based on the verified headers
	ProcessHeaders(headers []*types.Header) error

	// GetBlockCreator retrieves the block creator (or signer) given the block header
	GetBlockCreator(header *types.Header) (types.Address, error)

	// PreStateCommit a hook to be called before finalizing state transition on inserting block
	PreStateCommit(header *types.Header, txn *state.Transition) error

	//	TODO: this seems to be for eth_syncing only
	// GetSyncProgression retrieves the current sync progression, if any
	GetSyncProgression() *progress.Progression

	// Initialize initializes the backend (e.g. setup data)
	Initialize() error

	// Start starts the backend and servers
	Start() error

	// Close closes the connection
	Close() error
}

// Config is the configuration for the backend
type Config struct {
	// Logger to be used by the backend
	Logger *log.Logger

	// Params are the params of the chain and the backend
	Params *chain.Params

	// Config defines specific configuration parameters for the backend
	Config map[string]interface{}

	// Path is the directory path for the backend protocol tos tore information
	Path string
}

type BackendParams struct {
	Context        context.Context
	Seal           bool
	Config         *Config
	TxPool         *txpool.TxPool
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
type Factory func(*BackendParams) (Backend, error)
