package consensus

import (
	"context"

	"github.com/umbracle/minimal/state"
	"github.com/umbracle/minimal/types"
)

/*
var err = fmt.Errorf("NoProof consensus only meant to be used to verify headers only")
*/

// NoProof is a consensus algorithm that validates everything
type NoProof struct {
}

// VerifyHeader verifies the header is correct
func (n *NoProof) VerifyHeader(parent *types.Header, header *types.Header, uncle, seal bool) error {
	return nil
}

// Seal seals the block
func (n *NoProof) Seal(ctx context.Context, block *types.Block) (*types.Block, error) {
	block.Header.ComputeHash()
	return block, nil
}

// Close closes the connection
func (n *NoProof) Close() error {
	return nil
}

func (n *NoProof) Prepare(parent *types.Header, header *types.Header) error {
	return nil
}

func (n *NoProof) Finalize(txn *state.Txn, block *types.Block) error {
	return nil
}
