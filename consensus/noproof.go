package consensus

import (
	"context"

	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/types"
)

// NoProof is a consensus algorithm that validates everything
type NoProof struct {
}

// VerifyHeader verifies the header is correct
func (n *NoProof) VerifyHeader(parent, header *types.Header, uncle, seal bool) error {
	return nil
}

// Prepare initializes the consensus fields of a block header according to the
// rules of a particular engine. The changes are executed inline.
func (n *NoProof) Prepare(header *types.Header) error {
	return nil
}

// Seal seals the block
func (n *NoProof) Seal(block *types.Block, ctx context.Context) (*types.Block, error) {
	block.Header.ComputeHash()

	return block, nil
}

// Close closes the connection
func (n *NoProof) Close() error {
	return nil
}

func (n *NoProof) Finalize(txn *state.Txn, block *types.Block) error {
	return nil
}
