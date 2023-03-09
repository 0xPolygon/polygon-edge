package jsonrpc

import (
	"github.com/0xPolygon/polygon-edge/types"
)

// bridgeStore interface provides access to the methods needed by bridge endpoint
type bridgeStore interface {
	GenerateExitProof(exitID uint64) (types.Proof, error)
	GetStateSyncProof(stateSyncID uint64) (types.Proof, error)
}

// Bridge is the bridge jsonrpc endpoint
type Bridge struct {
	store bridgeStore
}

// GenerateExitProof generates exit proof for given exit event
func (b *Bridge) GenerateExitProof(exitID argUint64) (interface{}, error) {
	return b.store.GenerateExitProof(uint64(exitID))
}

// GetStateSyncProof retrieves the StateSync proof
func (b *Bridge) GetStateSyncProof(stateSyncID argUint64) (interface{}, error) {
	return b.store.GetStateSyncProof(uint64(stateSyncID))
}
