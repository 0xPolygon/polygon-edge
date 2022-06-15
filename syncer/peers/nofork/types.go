package nofork

import (
	"math/big"

	"github.com/0xPolygon/polygon-edge/blockchain"
	"github.com/0xPolygon/polygon-edge/network"
	"github.com/0xPolygon/polygon-edge/network/event"
	"github.com/0xPolygon/polygon-edge/types"
	lp2pNetwork "github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
)

type Blockchain interface {
	// Get latest header
	Header() *types.Header
	// Get block from chain
	GetBlockByNumber(uint64, bool) (*types.Block, bool)
	// Subscribe new block event
	SubscribeEvents() blockchain.Subscription
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
	// NewStream open a stream to communicate with the peer
	NewStream(string, peer.ID) (lp2pNetwork.Stream, error)
}
