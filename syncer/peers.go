package syncer

import (
	"math/big"
	"sync"
)

type NoForkPeer struct {
	// identifier
	ID string
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

	for _, p := range peers {
		peerMap.Put(p)
	}

	return peerMap
}

func (m *PeerMap) Put(peer *NoForkPeer) {
	m.Store(peer.ID, peer)
}

// Remove removes a peer from heap if it exists
func (m *PeerMap) Remove(peerID string) {
	m.Delete(peerID)
}

// BestPeer returns the top of heap
func (m *PeerMap) BestPeer(skipMap map[string]bool) *NoForkPeer {
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
