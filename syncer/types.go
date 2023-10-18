package syncer

import (
	"context"
	"math/big"
	"time"

	rawGrpc "google.golang.org/grpc"

	"github.com/0xPolygon/polygon-edge/blockchain"
	"github.com/0xPolygon/polygon-edge/helper/progress"
	"github.com/0xPolygon/polygon-edge/network"
	"github.com/0xPolygon/polygon-edge/network/event"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/libp2p/go-libp2p/core/peer"
	"google.golang.org/protobuf/proto"
)

const syncerMetrics = "syncer"

type Blockchain interface {
	// SubscribeEvents subscribes new blockchain event
	SubscribeEvents() blockchain.Subscription
	// UnsubscribeEvents unsubscribes from new blockchain event
	UnsubscribeEvents(blockchain.Subscription)
	// Header returns get latest header
	Header() *types.Header
	// GetBlockByNumber returns block by number
	GetBlockByNumber(uint64, bool) (*types.Block, bool)
	// VerifyFinalizedBlock verifies finalized block
	VerifyFinalizedBlock(block *types.Block) (*types.FullBlock, error)
	// WriteBlock writes a given block to chain
	WriteBlock(*types.Block, string) error
	// WriteFullBlock writes a given block to chain and saves its receipts to cache
	WriteFullBlock(*types.FullBlock, string) error
}

type Network interface {
	// AddrInfo returns Network Info
	AddrInfo() *peer.AddrInfo
	// RegisterProtocol registers gRPC service
	RegisterProtocol(string, network.Protocol)
	// Peers returns current connected peers
	Peers() []*network.PeerConnInfo
	// SubscribeCh returns a channel of peer event
	SubscribeCh(context.Context) (<-chan *event.PeerEvent, error)
	// GetPeerDistance returns the distance between the node and given peer
	GetPeerDistance(peer.ID) *big.Int
	// NewProtoConnection opens up a new stream on the set protocol to the peer,
	// and returns a reference to the connection
	NewProtoConnection(protocol string, peerID peer.ID) (*rawGrpc.ClientConn, error)
	// NewTopic Creates New Topic for gossip
	NewTopic(protoID string, obj proto.Message) (*network.Topic, error)
	// IsConnected returns the node is connecting to the peer associated with the given ID
	IsConnected(peerID peer.ID) bool
	// SaveProtocolStream saves stream
	SaveProtocolStream(protocol string, stream *rawGrpc.ClientConn, peerID peer.ID)
	// CloseProtocolStream closes stream
	CloseProtocolStream(protocol string, peerID peer.ID) error
}

type Syncer interface {
	// Start starts syncer processes
	Start() error
	// Close terminates syncer process
	Close() error
	// GetSyncProgression returns sync progression
	GetSyncProgression() *progress.Progression
	// HasSyncPeer returns whether syncer has the peer syncer can sync with
	HasSyncPeer() bool
	// Sync starts routine to sync blocks
	Sync(func(*types.FullBlock) bool) error
}

type Progression interface {
	// StartProgression starts progression
	StartProgression(startingBlock uint64, subscription blockchain.Subscription)
	// UpdateHighestProgression updates highest block number
	UpdateHighestProgression(highestBlock uint64)
	// GetProgression returns Progression
	GetProgression() *progress.Progression
	// StopProgression finishes progression
	StopProgression()
}

type SyncPeerService interface {
	// Start starts server
	Start()
	// Close terminates running processes for SyncPeerService
	Close() error
}

type SyncPeerClient interface {
	// Start processes for SyncPeerClient
	Start() error
	// Close terminates running processes for SyncPeerClient
	Close()
	// GetPeerStatus fetches peer status
	GetPeerStatus(id peer.ID) (*NoForkPeer, error)
	// GetConnectedPeerStatuses fetches the statuses of all connecting peers
	GetConnectedPeerStatuses() []*NoForkPeer
	// GetBlocks returns a stream of blocks from given height to peer's latest
	GetBlocks(peer.ID, uint64, time.Duration) (<-chan *types.Block, error)
	// GetPeerStatusUpdateCh returns a channel of peer's status update
	GetPeerStatusUpdateCh() <-chan *NoForkPeer
	// GetPeerConnectionUpdateEventCh returns peer's connection change event
	GetPeerConnectionUpdateEventCh() <-chan *event.PeerEvent
	// CloseStream close a stream
	CloseStream(peerID peer.ID) error
	// DisablePublishingPeerStatus disables publishing status in syncer topic
	DisablePublishingPeerStatus()
	// EnablePublishingPeerStatus enables publishing status in syncer topic
	EnablePublishingPeerStatus()
}
