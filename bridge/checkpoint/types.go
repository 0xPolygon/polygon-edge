package checkpoint

import "github.com/0xPolygon/polygon-edge/types"

type CheckpointData struct {
	Proposer        types.Address
	Start           uint64
	End             uint64
	RootHash        types.Hash // merkle root of the block hashes
	AccountRootHash types.Hash // merkle root of the validators
	Timestamp       uint64
}

func (c *CheckpointData) Hash() types.Hash {
	// TODO
	return types.Hash{}
}
