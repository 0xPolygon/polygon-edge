package checkpoint

import "github.com/0xPolygon/polygon-edge/types"

type RootChainContractClient interface {
	GetLastChildBlock() (uint64, error)
	GetCurrentHeaderBlock() (uint64, error)
	SubmitCheckpoint(data *CheckpointData, signatures [][]byte) (types.Hash, error)
}
