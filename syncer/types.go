package syncer

import (
	"context"
	"math/big"

	"github.com/0xPolygon/polygon-edge/blockchain"
	"github.com/0xPolygon/polygon-edge/helper/progress"
	"github.com/0xPolygon/polygon-edge/network"
	"github.com/0xPolygon/polygon-edge/network/event"
	"github.com/0xPolygon/polygon-edge/types"
	lp2pNetwork "github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"google.golang.org/protobuf/proto"
)

type Blockchain interface {
	// Subscribe new block event
	SubscribeEvents() blockchain.Subscription
	// Get latest header
	Header() *types.Header
	// Get Block by number
	GetBlockByNumber(uint64, bool) (*types.Block, bool)
	// Verify fetched block
	VerifyFinalizedBlock(*types.Block) error
	// Write block to chain
	WriteBlock(*types.Block) error
}

type Network interface {
	// AddrInfo returns Network Info
	AddrInfo() *peer.AddrInfo
	// Register gRPC service
	RegisterProtocol(string, network.Protocol)
	// Get current connected peers
	Peers() []*network.PeerConnInfo
	// Subscribe peer added/removed events
	SubscribeCh() (<-chan *event.PeerEvent, error)
	// Get distance between node and peer
	GetPeerDistance(peer.ID) *big.Int
	// NewStream opens a stream to communicate with the peer
	NewStream(string, peer.ID) (lp2pNetwork.Stream, error)
	// NewTopic Creates New Topic for gossip
	NewTopic(protoID string, obj proto.Message) (*network.Topic, error)
	IsConnected(peerID peer.ID) bool
}

type Syncer interface {
	Start() error
	GetSyncProgression() *progress.Progression
	HasSyncPeer() bool
	BulkSync(context.Context, func(*types.Block)) error
	WatchSync(context.Context, func(*types.Block) bool) error
}

type Progression interface {
	StartProgression(startingBlock uint64, subscription blockchain.Subscription)
	UpdateHighestProgression(highestBlock uint64)
	GetProgression() *progress.Progression
	StopProgression()
}

type SyncPeerService interface {
	Start()
}

type SyncPeerClient interface {
	Start() error
	Close()
	GetPeerStatus(id peer.ID) (*NoForkPeer, error)
	GetConnectedPeerStatuses() []*NoForkPeer
	GetBlocks(context.Context, peer.ID, uint64) (<-chan *types.Block, error)
	GetBlock(context.Context, peer.ID, uint64) (*types.Block, error)
	GetPeerStatusUpdateCh() <-chan *NoForkPeer
	GetPeerConnectionUpdateEventCh() <-chan *event.PeerEvent
}
