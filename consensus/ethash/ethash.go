package ethash

import (
	"bytes"
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/core/types"
	lru "github.com/hashicorp/golang-lru"
	"github.com/umbracle/minimal/chain"
	"github.com/umbracle/minimal/consensus"
	"github.com/umbracle/minimal/state"
)

var (
	two256 = new(big.Int).Exp(big.NewInt(2), big.NewInt(256), big.NewInt(0))
)

// Ethash is the ethash consensus algorithm
type Ethash struct {
	config  *chain.Params
	cache   *lru.Cache
	fakePow bool
}

// Factory is the factory method to create an Ethash consensus
func Factory(ctx context.Context, config *consensus.Config) (consensus.Consensus, error) {
	cache, _ := lru.New(2)
	e := &Ethash{
		config: config.Params,
		cache:  cache,
	}
	return e, nil
}

// VerifyHeader verifies the header is correct
func (e *Ethash) VerifyHeader(parent *types.Header, header *types.Header, uncle, seal bool) error {
	headerNum := header.Number.Uint64()
	parentNum := parent.Number.Uint64()

	if headerNum != parentNum+1 {
		return fmt.Errorf("header and parent are non sequential")
	}
	if header.Time.Cmp(parent.Time) <= 0 {
		return fmt.Errorf("incorrect timestamp")
	}

	if header.Difficulty.Sign() <= 0 {
		return fmt.Errorf("difficulty cannot be negative")
	}

	if uint64(len(header.Extra)) > chain.MaximumExtraDataSize {
		return fmt.Errorf("extradata is too long")
	}

	if uncle {
		if header.Time.Cmp(math.MaxBig256) > 0 {
			return fmt.Errorf("incorrect uncle timestamp")
		}
	} else {
		if header.Time.Cmp(big.NewInt(time.Now().Add(15*time.Second).Unix())) > 0 {
			return fmt.Errorf("future block")
		}
	}

	diff := e.CalcDifficulty(header.Time.Uint64(), parent)
	if diff.Cmp(header.Difficulty) != 0 {
		return fmt.Errorf("incorrect difficulty")
	}

	cap := uint64(0x7fffffffffffffff)
	if header.GasLimit > cap {
		return fmt.Errorf("incorrect gas limit")
	}

	if header.GasUsed > header.GasLimit {
		return fmt.Errorf("incorrect gas used")
	}

	gas := int64(parent.GasLimit) - int64(header.GasLimit)
	if gas < 0 {
		gas *= -1
	}

	limit := parent.GasLimit / chain.GasLimitBoundDivisor
	if uint64(gas) >= limit || header.GasLimit < chain.MinGasLimit {
		return fmt.Errorf("incorrect gas limit")
	}

	if !e.fakePow {
		// Verify the seal
		number := header.Number.Uint64()
		cache := e.getCache(number)

		digest, result := cache.hashimoto(sealHash(header).Bytes(), header.Nonce.Uint64())

		if !bytes.Equal(header.MixDigest[:], digest) {
			return fmt.Errorf("incorrect digest")
		}

		target := new(big.Int).Div(two256, header.Difficulty)
		if new(big.Int).SetBytes(result).Cmp(target) > 0 {
			return fmt.Errorf("incorrect pow")
		}
	}

	return nil
}

func (e *Ethash) getCache(blockNumber uint64) *Cache {
	epoch := blockNumber / uint64(epochLength)
	cache, ok := e.cache.Get(epoch)
	if ok {
		return cache.(*Cache)
	}

	cc := newCache(int(epoch))
	e.cache.Add(epoch, cc)
	return cc
}

// SetFakePow sets the fakePow flag to true, only used on tests.
func (e *Ethash) SetFakePow() {
	e.fakePow = true
}

// CalcDifficulty calculates the difficulty at a given time.
func (e *Ethash) CalcDifficulty(time uint64, parent *types.Header) *big.Int {
	next := parent.Number.Uint64() + 1
	switch {
	case e.config.Forks.IsConstantinople(next):
		return MetropolisDifficulty(time, parent, ConstantinopleBombDelay)

	case e.config.Forks.IsByzantium(next):
		return MetropolisDifficulty(time, parent, ByzantiumBombDelay)

	case e.config.Forks.IsHomestead(next):
		return HomesteadDifficulty(time, parent)

	default:
		return FrontierDifficulty(time, parent)
	}
}

// Author checks the author of the header
func (e *Ethash) Author(header *types.Header) (common.Address, error) {
	return common.Address{}, nil
}

// Seal seals the block
func (e *Ethash) Seal(ctx context.Context, block *types.Block) (*types.Block, error) {
	return nil, nil
}

// Prepare runs before processing the head during mining.
func (e *Ethash) Prepare(parent *types.Header, header *types.Header) error {
	return nil
}

var (
	big8  = big.NewInt(8)
	big32 = big.NewInt(32)
)

// Block rewards at different forks
var (
	// FrontierBlockReward is the block reward for the Frontier fork
	FrontierBlockReward = big.NewInt(5e+18)

	// ByzantiumBlockReward is the block reward for the Byzantium fork
	ByzantiumBlockReward = big.NewInt(3e+18)

	// ConstantinopleBlockReward is the block reward for the Constantinople fork
	ConstantinopleBlockReward = big.NewInt(2e+18)
)

// Finalize runs after the block has been processed
func (e *Ethash) Finalize(txn *state.Txn, block *types.Block) error {
	number := block.Number()

	var blockReward *big.Int
	switch num := number.Uint64(); {
	case e.config.Forks.IsConstantinople(num):
		blockReward = ConstantinopleBlockReward
	case e.config.Forks.IsByzantium(num):
		blockReward = ByzantiumBlockReward
	default:
		blockReward = FrontierBlockReward
	}

	reward := new(big.Int).Set(blockReward)

	r := new(big.Int)
	for _, uncle := range block.Uncles() {
		r.Add(uncle.Number, big8)
		r.Sub(r, number)
		r.Mul(r, blockReward)
		r.Div(r, big8)

		txn.AddBalance(uncle.Coinbase, r)

		r.Div(blockReward, big32)
		reward.Add(reward, r)
	}

	txn.AddBalance(block.Coinbase(), reward)
	return nil
}

// Close closes the connection
func (e *Ethash) Close() error {
	return nil
}
