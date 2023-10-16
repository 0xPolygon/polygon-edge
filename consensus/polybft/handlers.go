package polybft

import (
	"github.com/0xPolygon/polygon-edge/consensus/polybft/validator"
	"github.com/0xPolygon/polygon-edge/types"
	bolt "go.etcd.io/bbolt"
)

type PostBlockRequest struct {
	// FullBlock is a reference of the executed block
	FullBlock *types.FullBlock
	// Epoch is the epoch number of the executed block
	Epoch uint64
	// IsEpochEndingBlock indicates if this was the last block of given epoch
	IsEpochEndingBlock bool
	// DBTx is the opened transaction on state store (in our case boltDB)
	// used to save necessary data on PostBlock
	DBTx *bolt.Tx
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

	// DBTx is the opened transaction on state store (in our case boltDB)
	// used to save necessary data on PostEpoch
	DBTx *bolt.Tx
}
