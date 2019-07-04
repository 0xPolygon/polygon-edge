package ethash

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"math/big"
	"time"

	lru "github.com/hashicorp/golang-lru"
	"github.com/umbracle/minimal/chain"
	"github.com/umbracle/minimal/consensus"
	"github.com/umbracle/minimal/state"
	"github.com/umbracle/minimal/types"
)

var (
	two256 = new(big.Int).Exp(big.NewInt(2), big.NewInt(256), big.NewInt(0))
)

// Ethash is the ethash consensus algorithm
type Ethash struct {
	config  *chain.Params
	cache   *lru.Cache
	fakePow bool

	// tmp is the seal hash tmp variable
	tmp []byte
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
	headerNum := header.Number
	parentNum := parent.Number

	if headerNum != parentNum+1 {
		return fmt.Errorf("header and parent are non sequential")
	}
	if header.Timestamp <= parent.Timestamp {
		return fmt.Errorf("incorrect timestamp")
	}

	if uint64(len(header.ExtraData)) > chain.MaximumExtraDataSize {
		return fmt.Errorf("extradata is too long")
	}

	if uncle {
		// TODO
	} else {
		if int64(header.Timestamp) > time.Now().Add(15*time.Second).Unix() {
			return fmt.Errorf("future block")
		}
	}

	diff := e.CalcDifficulty(int64(header.Timestamp), parent)
	if diff != header.Difficulty {
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
		number := header.Number
		cache := e.getCache(number)

		nonce := binary.BigEndian.Uint64(header.Nonce[:])
		hash := e.sealHash(header)
		digest, result := cache.hashimoto(hash, nonce)

		if !bytes.Equal(header.MixHash[:], digest) {
			return fmt.Errorf("incorrect digest")
		}

		target := new(big.Int).Div(two256, new(big.Int).SetUint64(header.Difficulty))
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
func (e *Ethash) CalcDifficulty(time int64, parent *types.Header) uint64 {
	next := parent.Number + 1

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
func (e *Ethash) Author(header *types.Header) (types.Address, error) {
	return types.Address{}, nil
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
	numberBigInt := big.NewInt(int64(number))

	var blockReward *big.Int
	switch num := number; {
	case e.config.Forks.IsConstantinople(num):
		blockReward = ConstantinopleBlockReward
	case e.config.Forks.IsByzantium(num):
		blockReward = ByzantiumBlockReward
	default:
		blockReward = FrontierBlockReward
	}

	reward := new(big.Int).Set(blockReward)

	r := new(big.Int)
	for _, uncle := range block.Uncles {
		r.Add(big.NewInt(int64(uncle.Number)), big8)
		r.Sub(r, numberBigInt)
		r.Mul(r, blockReward)
		r.Div(r, big8)

		txn.AddBalance(uncle.Miner, r)

		r.Div(blockReward, big32)
		reward.Add(reward, r)
	}

	txn.AddBalance(block.Header.Miner, reward)
	return nil
}

// Close closes the connection
func (e *Ethash) Close() error {
	return nil
}
