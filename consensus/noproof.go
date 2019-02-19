package consensus

import (
	"context"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/umbracle/minimal/state"
)

var err = fmt.Errorf("NoProof consensus only meant to be used to verify headers only")

// NoProof is a consensus algorithm that validates everything
type NoProof struct {
}

// VerifyHeader verifies the header is correct
func (n *NoProof) VerifyHeader(parent *types.Header, header *types.Header, uncle, seal bool) error {
	return nil
}

// Author checks the author of the header
func (n *NoProof) Author(header *types.Header) (common.Address, error) {
	return common.Address{}, err
}

// Seal seals the block
func (n *NoProof) Seal(ctx context.Context, block *types.Block) (*types.Block, error) {
	return nil, err
}

// Close closes the connection
func (n *NoProof) Close() error {
	return err
}

func (n *NoProof) Prepare(parent *types.Header, header *types.Header) error {
	return nil
}

func (n *NoProof) Finalize(txn *state.Txn, block *types.Block) error {
	return nil
}
