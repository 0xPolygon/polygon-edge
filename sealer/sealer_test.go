package sealer

import (
	"context"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/umbracle/minimal/state"
)

type boolSealer struct {
}

func (b *boolSealer) VerifyHeader(parent *types.Header, header *types.Header, uncle, seal bool) error {
	return nil
}

func (b *boolSealer) Author(header *types.Header) (common.Address, error) {
	return common.Address{}, nil
}

func (b *boolSealer) Seal(ctx context.Context, block *types.Block) error {
	return nil
}

func (b *boolSealer) Finalize(txn *state.Txn, block *types.Block) error {
	return nil
}

func (b *boolSealer) Close() error {
	return nil
}
