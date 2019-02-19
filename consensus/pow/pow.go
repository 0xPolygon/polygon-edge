package pow

import (
	"context"
	crand "crypto/rand"
	"fmt"
	"math"
	"math/big"
	"math/rand"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/umbracle/minimal/consensus"
	"github.com/umbracle/minimal/state"
)

var (
	two256 = new(big.Int).Exp(big.NewInt(2), big.NewInt(256), big.NewInt(0))
)

// Pow is a vanilla proof-of-work engine with a fixed difficulty
type Pow struct {
	difficulty *big.Int
}

func Factory(ctx context.Context, config *consensus.Config) (consensus.Consensus, error) {
	return &Pow{}, nil
}

func (p *Pow) VerifyHeader(parent *types.Header, header *types.Header, uncle, seal bool) error {
	if header.Time.Cmp(parent.Time) <= 0 {
		return fmt.Errorf("timestamp lower or equal than parent")
	}
	if diff := new(big.Int).Sub(header.Number, parent.Number); diff.Cmp(big.NewInt(1)) != 0 {
		return fmt.Errorf("invalid sequence")
	}
	target := new(big.Int).Div(two256, p.difficulty)
	if big.NewInt(1).SetBytes(header.Hash().Bytes()).Cmp(target) > 0 {
		return fmt.Errorf("nonce is not correct")
	}
	return nil
}

func (p *Pow) Author(header *types.Header) (common.Address, error) {
	return common.Address{}, nil
}

func (p *Pow) Seal(ctx context.Context, block *types.Block) (*types.Block, error) {
	header := block.Header()

	seed, err := crand.Int(crand.Reader, big.NewInt(math.MaxInt64))
	if err != nil {
		return nil, err
	}

	rand := rand.New(rand.NewSource(seed.Int64()))
	nonce := uint64(rand.Int63())

	target := new(big.Int).Div(two256, p.difficulty)

	for {
		header.Nonce = types.EncodeNonce(uint64(nonce))
		hash := header.Hash()

		if big.NewInt(1).SetBytes(hash.Bytes()).Cmp(target) < 0 {
			break
		}
		nonce++
	}
	return types.NewBlockWithHeader(header), nil
}

func (p *Pow) Prepare(parent *types.Header, header *types.Header) error {
	return nil
}

func (p *Pow) Finalize(txn *state.Txn, block *types.Block) error {
	return nil
}

func (p *Pow) Close() error {
	return nil
}
