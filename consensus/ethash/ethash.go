package ethash

import (
	"fmt"
	"math/big"
	"time"

	"github.com/umbracle/minimal/chain"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
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
type EthHash struct {
	config *chain.Params
}

// NewEthHash creates a new ethash consensus
func NewEthHash(config *chain.Params) *EthHash {
	return &EthHash{config}
}

// VerifyHeader verifies the header is correct
func (e *EthHash) VerifyHeader(parent *types.Header, header *types.Header, seal bool) error {
	// Ensure that the header's extra-data section is of a reasonable size
	if uint64(len(header.Extra)) > chain.MaximumExtraDataSize {
		return fmt.Errorf("Extra data too long")
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

func (e *EthHash) verifySeal(header *types.Header) error {
	return nil
}

// Author checks the author of the header
func (e *EthHash) Author(header *types.Header) (common.Address, error) {
	return common.Address{}, nil
}

// Seal seals the block
func (e *EthHash) Seal(block *types.Block) error {
	return nil
}

func (e *EthHash) CalcDifficulty(time uint64, parent *types.Header) *big.Int {
	next := parent.Number.Uint64() + 1
	switch {
	case e.config.Forks.Constantinople.Active(next):
		return calcDifficultyConstantinople(time, parent)
	case e.config.Forks.Byzantium.Active(next):
		return calcDifficultyByzantium(time, parent)
	case e.config.Forks.Homestead.Active(next):
		return calcDifficultyHomestead(time, parent)
	default:
		return calcDifficultyFrontier(time, parent)
	}
}

// Close closes the connection
func (e *EthHash) Close() error {
	return nil
}
