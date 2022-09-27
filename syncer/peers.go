package syncer

import (
	"math/big"
	"sync"

	"github.com/libp2p/go-libp2p/core/peer"
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

	peerMap.Put(peers...)

	return peerMap
}

func (m *PeerMap) Put(peers ...*NoForkPeer) {
	for _, peer := range peers {
		m.Store(peer.ID.String(), peer)
	}
}

// Remove removes a peer from heap if it exists
func (m *PeerMap) Remove(peerID peer.ID) {
	m.Delete(peerID.String())
}

// BestPeer returns the top of heap
func (m *PeerMap) BestPeer(skipMap map[peer.ID]bool) *NoForkPeer {
	var bestPeer *NoForkPeer

	m.Range(func(key, value interface{}) bool {
		peer, _ := value.(*NoForkPeer)

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
