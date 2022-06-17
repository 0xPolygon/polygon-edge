package syncer

import (
	"math/big"
	"sync"

	"github.com/libp2p/go-libp2p-core/peer"
)

type NoForkPeer struct {
	// identifier
	ID peer.ID
	// peer's latest block number
	Number uint64
	// peer's distance
	Distance *big.Int
}

func (p *NoForkPeer) IsBetter(t *NoForkPeer) bool {
	if p.Number != t.Number {
		return p.Number > t.Number
	}

	return p.Distance.Cmp(t.Distance) < 0
}

type PeerMap struct {
	sync.Map
}

func NewPeerMap(peers []*NoForkPeer) *PeerMap {
	peerMap := new(PeerMap)

	peerMap.PutPeers(peers)

	return peerMap
}

func (m *PeerMap) PutPeers(peers []*NoForkPeer) {
	for _, p := range peers {
		m.Put(p)
	}
}

func (m *PeerMap) Put(peer *NoForkPeer) {
	m.Store(peer.ID, peer)
}

// Remove removes a peer from heap if it exists
func (m *PeerMap) Remove(peerID string) {
	m.Delete(peerID)
}

// BestPeer returns the top of heap
func (m *PeerMap) BestPeer(skipMap map[peer.ID]bool) *NoForkPeer {
	var bestPeer *NoForkPeer

	m.Range(func(key, value interface{}) bool {
		peer, ok := value.(*NoForkPeer)
		if !ok {
			// shouldn't reach here
			return false
		}

		if skipMap != nil && skipMap[peer.ID] {
			return true
		}

		if bestPeer == nil || peer.IsBetter(bestPeer) {
			bestPeer = peer
		}

		return true
	})

	return bestPeer
}
