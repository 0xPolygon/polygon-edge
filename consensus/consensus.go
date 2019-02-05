package consensus

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/umbracle/minimal/state"
)

// Consensus is the interface for consensus
type Consensus interface {
	// VerifyHeader verifies the header is correct
	VerifyHeader(parent *types.Header, header *types.Header, uncle, seal bool) error

	// Author checks the author of the header
	Author(header *types.Header) (common.Address, error)

	// Seal seals the block
	Seal(block *types.Block) error

	// Finalize do
	Finalize(txn *state.Txn, block *types.Block) error

	// Close closes the connection
	Close() error
}
