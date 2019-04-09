package ethash

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/big"
	"math/rand"
	"runtime"
	"sync"
	"time"

	"github.com/umbracle/minimal/chain"
	"github.com/umbracle/minimal/consensus"
	"github.com/umbracle/minimal/state"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/core/types"
)

var (
	errLargeBlockTime    = errors.New("timestamp too big")
	errTooManyUncles     = errors.New("too many uncles")
	errInvalidDifficulty = errors.New("non-positive difficulty")
	errInvalidMixDigest  = errors.New("invalid mix digest")
	errInvalidPoW        = errors.New("invalid proof-of-work")
	errFutureBlock       = errors.New("future block")
)

var (
	// FrontierBlockReward is the block reward in wei for successfully mining a block
	FrontierBlockReward = big.NewInt(5e+18)
	// ByzantiumBlockReward is the block reward in wei for successfully mining a block upward from Byzantium
	ByzantiumBlockReward = big.NewInt(3e+18)
	// ConstantinopleBlockReward is the block reward in wei for successfully mining a block upward from Constantinople
	ConstantinopleBlockReward = big.NewInt(2e+18)
	// maxUncles is the maximum number of uncles allowed in a single block
	maxUncles = 2
	// allowedFutureBlockTime is the max time from current time allowed for blocks, before they're considered future blocks
	allowedFutureBlockTime = 15 * time.Second
)

// EthHash consensus algorithm
type Ethash struct {
	config  *chain.Params
	lock    sync.Mutex
	rand    *rand.Rand
	threads int

	DatasetDir     string
	DatasetsOnDisk int

	CacheDir     string
	CachesOnDisk int

	Test bool

	FakePow  bool
	caches   *lru // In memory caches to avoid regenerating too often
	datasets *lru // In memory datasets to avoid regenerating too often
}

func Factory(ctx context.Context, config *consensus.Config) (consensus.Consensus, error) {
	e := &Ethash{
		config:   config.Params,
		caches:   newlru("cache", 1, newCache),
		datasets: newlru("dataset", 1, newDataset),
	}
	return e, nil
}

// NewEthHash creates a new ethash consensus
func NewEthHash(config *chain.Params, FakePow bool) *Ethash {
	e := &Ethash{
		config:   config,
		caches:   newlru("cache", 1, newCache),
		datasets: newlru("dataset", 1, newDataset),
		FakePow:  FakePow,
	}
	return e
}

// VerifyHeader verifies the header is correct
func (e *Ethash) VerifyHeader(parent *types.Header, header *types.Header, uncle, seal bool) error {
	// Ensure that the header's extra-data section is of a reasonable size
	if uint64(len(header.Extra)) > chain.MaximumExtraDataSize {
		return fmt.Errorf("Extra data too long")
	}

	if uncle {
		if header.Time.Cmp(math.MaxBig256) > 0 {
			return errLargeBlockTime
		}
	} else {
		if header.Time.Cmp(big.NewInt(time.Now().Add(allowedFutureBlockTime).Unix())) > 0 {
			return errFutureBlock
		}
	}

	if header.Time.Cmp(parent.Time) <= 0 {
		return fmt.Errorf("timestamp lower or equal than parent")
	}
	// Verify the block's difficulty based in it's timestamp and parent's difficulty
	expected := e.CalcDifficulty(header.Time.Uint64(), parent)
	if expected.Cmp(header.Difficulty) != 0 {
		return fmt.Errorf("difficulty not correct: expected %d but found %d", expected.Uint64(), header.Difficulty.Uint64())
	}
	// Verify that the gas limit is <= 2^63-1
	cap := uint64(0x7fffffffffffffff)
	if header.GasLimit > cap {
		return fmt.Errorf("gas limit not correct: have %v, want %v", header.GasLimit, cap)
	}
	// Verify that the gasUsed is <= gasLimit
	if header.GasUsed > header.GasLimit {
		return fmt.Errorf("gas used not correct: have %v, want %v", header.GasUsed, header.GasLimit)
	}
	// Verify that the gas limit remains within allowed bounds
	diff := int64(parent.GasLimit) - int64(header.GasLimit)
	if diff < 0 {
		diff *= -1
	}
	limit := parent.GasLimit / chain.GasLimitBoundDivisor

	if uint64(diff) >= limit || header.GasLimit < chain.MinGasLimit {
		return fmt.Errorf("gas limit not correct")
	}
	// Verify that the block number is parent's +1
	if diff := new(big.Int).Sub(header.Number, parent.Number); diff.Cmp(big.NewInt(1)) != 0 {
		return fmt.Errorf("invalid sequence")
	}

	// Verify the engine specific seal securing the block
	if seal {
		if err := e.verifySeal(header); err != nil {
			return err
		}
	}
	return nil
}

var (
	big8  = big.NewInt(8)
	big32 = big.NewInt(32)
)

func (e *Ethash) Finalize(txn *state.Txn, block *types.Block) error {
	number := block.Number()

	// Select the correct block reward based on chain progression
	blockReward := FrontierBlockReward
	if e.config.Forks.IsByzantium(number.Uint64()) {
		blockReward = ByzantiumBlockReward
	}
	if e.config.Forks.IsConstantinople(number.Uint64()) {
		blockReward = ConstantinopleBlockReward
	}

	// Accumulate the rewards for the miner and any included uncles
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

func (e *Ethash) verifySeal(header *types.Header) error {
	if e.FakePow {
		return nil
	}

	// Ensure that we have a valid difficulty for the block
	if header.Difficulty.Sign() <= 0 {
		return errInvalidDifficulty
	}
	// Recompute the digest and PoW values
	number := header.Number.Uint64()

	var (
		digest []byte
		result []byte
	)

	cache := e.cache(number)

	size := datasetSize(number)
	digest, result = hashimotoLight(size, cache.cache, e.sealHash(header).Bytes(), header.Nonce.Uint64())

	// Caches are unmapped in a finalizer. Ensure that the cache stays alive
	// until after the call to hashimotoLight so it's not unmapped while being used.
	runtime.KeepAlive(cache)

	// Verify the calculated values against the ones provided in the header
	if !bytes.Equal(header.MixDigest[:], digest) {
		return errInvalidMixDigest
	}
	target := new(big.Int).Div(two256, header.Difficulty)
	if new(big.Int).SetBytes(result).Cmp(target) > 0 {
		return errInvalidPoW
	}
	return nil
}

// Author checks the author of the header
func (e *Ethash) Author(header *types.Header) (common.Address, error) {
	return common.Address{}, nil
}

func (e *Ethash) CalcDifficulty(time uint64, parent *types.Header) *big.Int {
	next := parent.Number.Uint64() + 1
	switch {
	case e.config.Forks.IsConstantinople(next):
		return calcDifficultyConstantinople(time, parent)
	case e.config.Forks.IsByzantium(next):
		return calcDifficultyByzantium(time, parent)
	case e.config.Forks.IsHomestead(next):
		return calcDifficultyHomestead(time, parent)
	default:
		return calcDifficultyFrontier(time, parent)
	}
}

// Close closes the connection
func (e *Ethash) Close() error {
	return nil
}
