package ethash

import (
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
	"github.com/umbracle/minimal/storage"
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
	db *storage.Storage
}

// NewEthHash creates a new ethash consensus
func NewEthHash(db *storage.Storage) *EthHash {
	return &EthHash{db}
}

// VerifyHeader verifies the header is correct
func (e *EthHash) VerifyHeader(header *types.Header, seal bool) error {
	parent, err := e.db.ReadHeader(header.ParentHash)
	if err != nil {
		return err
	}

	// Ensure that the header's extra-data section is of a reasonable size
	if uint64(len(header.Extra)) > params.MaximumExtraDataSize {
		return fmt.Errorf("")
	}
	// Verify the header's timestamp
	if header.Time.Cmp(big.NewInt(time.Now().Add(allowedFutureBlockTime).Unix())) > 0 {
		return fmt.Errorf("")
	}

	if header.Time.Cmp(parent.Time) <= 0 {
		return fmt.Errorf("")
	}

	/*
		// TODO
		// Verify the block's difficulty based in it's timestamp and parent's difficulty
		expected := ethash.CalcDifficulty(chain, header.Time.Uint64(), parent)

		if expected.Cmp(header.Difficulty) != 0 {
			return fmt.Errorf("")
		}
	*/

	// Verify that the gas limit is <= 2^63-1
	cap := uint64(0x7fffffffffffffff)
	if header.GasLimit > cap {
		return fmt.Errorf("")
	}
	// Verify that the gasUsed is <= gasLimit
	if header.GasUsed > header.GasLimit {
		return fmt.Errorf("")
	}

	// Verify that the gas limit remains within allowed bounds
	diff := int64(parent.GasLimit) - int64(header.GasLimit)
	if diff < 0 {
		diff *= -1
	}
	limit := parent.GasLimit / params.GasLimitBoundDivisor

	if uint64(diff) >= limit || header.GasLimit < params.MinGasLimit {
		return fmt.Errorf("")
	}
	// Verify that the block number is parent's +1
	if diff := new(big.Int).Sub(header.Number, parent.Number); diff.Cmp(big.NewInt(1)) != 0 {
		return fmt.Errorf("")
	}

	// Verify the engine specific seal securing the block
	if seal {
		if err := e.verifySeal(header); err != nil {
			return fmt.Errorf("")
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

// Close closes the connection
func (e *EthHash) Close() error {
	return nil
}
