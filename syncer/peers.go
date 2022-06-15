package syncer

import "sync"

type PeerHeap interface {
	Set(*Peer)
	BestPeer() *Peer
	Remove(id string)
}

type Peer struct {
	// identifier
	ID string
	// peer's latest block number
	Number uint64
	// peer's distance
	Distance uint64
}

type peerHeap struct {
	sync.RWMutex
	peers []*Peer
}

func newPeerHeap() PeerHeap {
	return &peerHeap{}
}

func (q *peerHeap) Set(peer *Peer) {
	// TODO
}

func (q *peerHeap) BestPeer() *Peer {
	// TODO
	return nil
}

func (q *peerHeap) Remove(id string) {
	// TODO
}
