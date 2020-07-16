package consensus

import (
	"context"
	"crypto/ecdsa"
	"io"
	"log"

	"github.com/0xPolygon/minimal/blockchain/storage"
	"github.com/0xPolygon/minimal/chain"
	"github.com/0xPolygon/minimal/state"
	"github.com/0xPolygon/minimal/types"
	"github.com/hashicorp/go-hclog"
)

// ChainReader defines a small collection of methods needed to access the local
// blockchain during header and/or uncle verification.
type ChainReader interface {
	// Config retrieves the blockchain's chain configuration.
	Config() *chain.Params

	// Executor retrieves the blockchain's executor.
	Executor() *state.Executor

	// CurrentHeader retrieves the current header from the local chain.
	CurrentHeader() (*types.Header, bool)

	// GetHeader retrieves a block header from the database by hash and number.
	GetHeader(hash types.Hash, number uint64) (*types.Header, bool)

	// GetHeaderByNumber retrieves a block header from the database by number.
	GetHeaderByNumber(number uint64) (*types.Header, bool)

	// GetHeaderByHash retrieves a block header from the database by its hash.
	GetHeaderByHash(hash types.Hash) (*types.Header, bool)

	// GetBlock retrieves a block from the database by hash and number.
	GetBlock(hash types.Hash, number uint64, full bool) (*types.Block, bool)
}

// Consensus is the interface for consensus
type Consensus interface {
	// VerifyHeader verifies the header is correct
	VerifyHeader(chain ChainReader, header *types.Header, uncle, seal bool) error

	//Prepare initializes the consensus fields of a block header according to the
	//rules of a particular engine. The changes are executed inline.
	Prepare(chain ChainReader, header *types.Header) error

	// Seal seals the block
	Seal(chain ChainReader, block *types.Block, ctx context.Context) (*types.Block, error)

	// Close closes the connection
	Close() error
}

type Msg struct {
	Code    uint64
	Size    uint32
	Payload io.Reader
}

// Handler should be implemented is the consensus needs to handle and send peer's message
type Handler interface {
	// NewChainHead handles a new head block comes
	NewChainHead() error

	//HandleMsg handles a message from peer
	HandleMsg(address types.Address, data Msg) (bool, error)

	// SetBroadcaster sets the broadcaster to send message to peers
	SetBroadcaster(Broadcaster)
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

// Istanbul is a consensus engine to avoid byzantine failure
type Istanbul interface {
	Consensus

	// Start starts the engine
	Start(chain ChainReader, currentBlock func(bool) *types.Block, hasBadBlock func(hash types.Hash) bool) error

	// Stop stops the engine
	Stop() error
}
