package pow

import (
	"context"
	crand "crypto/rand"
	"fmt"
	"math"
	"math/big"
	"math/rand"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/umbracle/minimal/consensus"
	"github.com/umbracle/minimal/state"
)

var (
	two256 = new(big.Int).Exp(big.NewInt(2), big.NewInt(256), big.NewInt(0))
)

// Pow is a vanilla proof-of-work engine
type Pow struct {
	min int
	max int
}

func Factory(ctx context.Context, config *consensus.Config) (consensus.Consensus, error) {
	return &Pow{min: 1000000, max: 1500000}, nil
}

func (p *Pow) VerifyHeader(parent *types.Header, header *types.Header, uncle, seal bool) error {
	if header.Time.Cmp(parent.Time) <= 0 {
		return fmt.Errorf("timestamp lower or equal than parent")
	}
	if diff := new(big.Int).Sub(header.Number, parent.Number); diff.Cmp(big.NewInt(1)) != 0 {
		return fmt.Errorf("invalid sequence")
	}
	localDiff := int(header.Difficulty.Int64())
	if localDiff < p.min {
		return fmt.Errorf("Difficulty not correct. '%d' <! '%d'", localDiff, p.min)
	}
	if localDiff > p.max {
		return fmt.Errorf("Difficulty not correct. '%d' >! '%d'", localDiff, p.min)
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

	target := new(big.Int).Div(two256, header.Difficulty)

	for {
		header.Nonce = types.EncodeNonce(uint64(nonce))
		hash := header.Hash()

		if big.NewInt(1).SetBytes(hash.Bytes()).Cmp(target) < 0 {
			break
		}
		nonce++

		if ctx.Err() != nil {
			return nil, nil
		}
	}

	return types.NewBlock(header, block.Transactions(), nil, nil), nil
}

func (p *Pow) Prepare(parent *types.Header, header *types.Header) error {
	header.Difficulty = big.NewInt(int64(randomInt(p.min, p.max)))
	return nil
}

func (p *Pow) Finalize(txn *state.Txn, block *types.Block) error {
	txn.AddBalance(block.Coinbase(), big.NewInt(1))
	return nil
}

func (p *Pow) Close() error {
	return nil
}

func randomInt(min, max int) int {
	rand.Seed(time.Now().UnixNano())
	return min + rand.Intn(max-min)
}
