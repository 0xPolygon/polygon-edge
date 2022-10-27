package jsonrpc

import (
	"github.com/0xPolygon/polygon-edge/types"
)

// bridgeStore interface provides access to the methods needed by bridge endpoint
type bridgeStore interface {
	GenerateExitProof(exitID, epoch, checkpointBlock uint64) ([]types.Hash, error)
}

// Bridge is the bridge jsonrpc endpoint
type Bridge struct {
	store bridgeStore
}

// GenerateExitProof generates exit proof for given exit event
func (b *Bridge) GenerateExitProof(exitID, epoch, checkpointBlock BlockNumber) (interface{}, error) {
	return b.store.GenerateExitProof(uint64(exitID), uint64(epoch), uint64(checkpointBlock))
}
