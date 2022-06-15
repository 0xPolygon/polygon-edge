package peers

import (
	"context"
	"time"

	"github.com/0xPolygon/polygon-edge/types"
)

const (
	SyncerProto           = "/syncer/0.2"
	DefaultRequestTimeout = 10 * time.Second
)

type BestPeer struct {
	ID     string // Peer ID
	Number uint64 // Latest Block Height
}

// SyncPeers manages a list of peers to sync blocks
type SyncPeers interface {
	// Start initialize gRPC server and get peers
	Start()
	// BestPeer returns the most suitable peer to sync
	BestPeer() *BestPeer
	// GetBlocks returns a channel of the blocks from the peer
	GetBlocks(ctx context.Context, peerID string, from uint64) (<-chan *types.Block, error)
	// GetBlock returns a block at the specified height from the peer
	GetBlock(ctx context.Context, peerID string, number uint64) (*types.Block, error)
}
