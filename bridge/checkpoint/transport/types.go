package transport

import "github.com/0xPolygon/polygon-edge/types"

type Checkpoint struct {
	Proposer        types.Address
	Start           uint64
	End             uint64
	RootHash        types.Hash
	AccountRootHash types.Hash
	Timestamp       uint64
}

type CheckpointProposalMessage struct {
	Checkpoint Checkpoint
	Signature  []byte
}

type AckMessage struct {
	CheckpointHash types.Hash
	TxHash         types.Hash
}

type NoAckMessage struct {
	Start uint64
	End   uint64
}
