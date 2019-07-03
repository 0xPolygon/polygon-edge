package pow

import (
	"context"
	crand "crypto/rand"
	"fmt"
	"math"
	"math/big"
	"math/rand"
	"time"

	"github.com/umbracle/minimal/consensus"
	"github.com/umbracle/minimal/state"
	"github.com/umbracle/minimal/types"
)

var (
	two256 = new(big.Int).Exp(big.NewInt(2), big.NewInt(256), big.NewInt(0))
)

// Pow is a vanilla proof-of-work engine
type Pow struct {
	min uint64
	max uint64
}

func Factory(ctx context.Context, config *consensus.Config) (consensus.Consensus, error) {
	return &Pow{min: 1000000, max: 1500000}, nil
}

func (p *Pow) VerifyHeader(parent *types.Header, header *types.Header, uncle, seal bool) error {
	if header.Timestamp <= parent.Timestamp {
		return fmt.Errorf("timestamp lower or equal than parent")
	}
	/*
		if diff := new(big.Int).Sub(header.Number, parent.Number); diff.Cmp(big.NewInt(1)) != 0 {
			return fmt.Errorf("invalid sequence")
		}
	*/
	if header.Difficulty < p.min {
		return fmt.Errorf("Difficulty not correct. '%d' <! '%d'", header.Difficulty, p.min)
	}
	if header.Difficulty > p.max {
		return fmt.Errorf("Difficulty not correct. '%d' >! '%d'", header.Difficulty, p.min)
	}
	return nil
}

func (p *Pow) Author(header *types.Header) (types.Address, error) {
	return types.Address{}, nil
}

func (p *Pow) Seal(ctx context.Context, block *types.Block) (*types.Block, error) {
	header := block.Header

	seed, err := crand.Int(crand.Reader, big.NewInt(math.MaxInt64))
	if err != nil {
		return nil, err
	}

	rand := rand.New(rand.NewSource(seed.Int64()))
	nonce := uint64(rand.Int63())

	target := new(big.Int).Div(two256, new(big.Int).SetUint64(header.Difficulty))
	for {
		// header.Nonce = types.EncodeNonce(uint64(nonce))

		hash := header.Hash()

		if big.NewInt(1).SetBytes(hash.Bytes()).Cmp(target) < 0 {
			break
		}
		nonce++

		if ctx.Err() != nil {
			return nil, nil
		}
	}

	block = &types.Block{
		Header:       header.Copy(),
		Transactions: block.Transactions,
	}
	return block, nil
}

func (p *Pow) Prepare(parent *types.Header, header *types.Header) error {
	header.Difficulty = randomInt(p.min, p.max)
	return nil
}

func (p *Pow) Finalize(txn *state.Txn, block *types.Block) error {
	txn.AddBalance(block.Header.Miner, big.NewInt(1))
	return nil
}

func (p *Pow) Close() error {
	return nil
}

func randomInt(min, max uint64) uint64 {
	rand.Seed(time.Now().UnixNano())
	return min + uint64(rand.Intn(int(max-min)))
}
