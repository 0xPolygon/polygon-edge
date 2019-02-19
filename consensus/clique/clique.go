package clique

import (
	"context"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/umbracle/minimal/consensus"
	"github.com/umbracle/minimal/state"
)

// Clique is a consensus algorithm for the clique protocol
type Clique struct {
}

func Factory(ctx context.Context, config *consensus.Config) (consensus.Consensus, error) {
	c := &Clique{}
	return c, nil
}

// VerifyHeader verifies the header is correct
func (c *Clique) VerifyHeader(parent *types.Header, header *types.Header, uncle, seal bool) error {
	return nil
}

// Author checks the author of the header
func (c *Clique) Author(header *types.Header) (common.Address, error) {
	return common.Address{}, nil
}

// Seal seals the block
func (c *Clique) Seal(ctx context.Context, block *types.Block) (*types.Block, error) {
	return nil, nil
}

// Close closes the connection
func (c *Clique) Close() error {
	return nil
}

func (c *Clique) Prepare(parent *types.Header, header *types.Header) error {
	return nil
}

func (c *Clique) Finalize(txn *state.Txn, block *types.Block) error {
	return nil
}
