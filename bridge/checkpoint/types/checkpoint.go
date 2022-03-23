package types

import (
	"github.com/0xPolygon/polygon-edge/types"
)

type Checkpoint struct {
	Proposer        types.Address
	Start           uint64
	End             uint64
	RootHash        types.Hash // merkle root of the block hashes
	AccountRootHash types.Hash // merkle root of the validators
	ChainID         uint64     // Edge Chain ID
}

func (c *Checkpoint) Hash() types.Hash {
	// TODO
	return types.Hash{}
}

type Ack struct {
	Epoch          uint64
	CheckpointHash types.Hash
	TxHash         types.Hash
}

func (c *Ack) Hash() types.Hash {
	// TODO
	return types.Hash{}
}

type NoAck struct {
	Epoch uint64
}

func (c *NoAck) Hash() types.Hash {
	// TODO
	return types.Hash{}
}
