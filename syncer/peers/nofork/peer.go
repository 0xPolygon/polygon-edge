package nofork

import (
	"math/big"

	"github.com/0xPolygon/polygon-edge/syncer/peers"
)

type NoForkPeer struct {
	// identifier
	id string
	// peer's latest block number
	Number uint64
	// peer's distance
	Distance *big.Int
}

func (p *NoForkPeer) ID() string {
	return p.id
}

func (p *NoForkPeer) Less(t peers.Peer) bool {
	target, ok := t.(*NoForkPeer)
	if !ok {
		return false
	}

	// sort by number
	if p.Number != target.Number {
		// reverse operator because go heap is min heap as default
		return p.Number > target.Number
	}

	return p.Distance.Cmp(target.Distance) < 0
}

func newHeap(nfPeers []*NoForkPeer) *peers.PeerHeap {
	ps := make([]peers.Peer, len(nfPeers))
	for i := range nfPeers {
		ps[i] = nfPeers[i]
	}

	return peers.NewPeerHeap(ps)
}
