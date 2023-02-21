package jsonrpc

import (
	"github.com/0xPolygon/polygon-edge/types"
)

// bridgeStore interface provides access to the methods needed by bridge endpoint
type bridgeStore interface {
	GenerateExitProof(exitID, epoch, checkpointBlock uint64) (types.Proof, error)
	GetStateSyncProof(stateSyncID uint64) (types.Proof, error)
}

// Bridge is the bridge jsonrpc endpoint
type Bridge struct {
	store bridgeStore
}

// GenerateExitProof generates exit proof for given exit event
func (b *Bridge) GenerateExitProof(exitID, epoch, checkpointBlock argUint64) (interface{}, error) {
	return b.store.GenerateExitProof(uint64(exitID), uint64(epoch), uint64(checkpointBlock))
}

// GetStateSyncProof retrieves the StateSync proof
func (b *Bridge) GetStateSyncProof(stateSyncID argUint64) (interface{}, error) {
	return b.store.GetStateSyncProof(uint64(stateSyncID))
}
