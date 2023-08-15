package common

import (
	"github.com/0xPolygon/polygon-edge/consensus/polybft/validator"
	"github.com/0xPolygon/polygon-edge/types"
)

// SystemState is an interface to interact with the consensus system contracts in the chain
type SystemState interface {
	// GetEpoch retrieves current epoch number from the smart contract
	GetEpoch() (uint64, error)

	// GetNextCommittedIndex retrieves next committed bridge state sync index
	GetNextCommittedIndex() (uint64, error)
}

type PostBlockRequest struct {
	// FullBlock is a reference of the executed block
	FullBlock *types.FullBlock

	// Epoch is the epoch number of the executed block
	Epoch uint64

	// IsEpochEndingBlock indicates if this was the last block of given epoch
	IsEpochEndingBlock bool
}

type PostEpochRequest struct {
	// NewEpochID is the id of the new epoch
	NewEpochID uint64

	// FirstBlockOfEpoch is the number of the epoch beginning block
	FirstBlockOfEpoch uint64

	// SystemState is the state of the governance smart contracts
	// after this block
	SystemState SystemState

	// ValidatorSet is the validator set for the new epoch
	ValidatorSet validator.ValidatorSet
}
