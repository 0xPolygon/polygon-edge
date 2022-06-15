package syncer

import (
	"container/heap"
	"sync"
)

type PeerHeap interface {
	Put(*Peer)
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
	peers     []*Peer
	lookupMap map[string]int
}

func newPeerHeap(peers []*Peer) PeerHeap {
	peerHeap := &peerHeap{
		peers:     peers,
		lookupMap: make(map[string]int, len(peers)),
	}

	heap.Init(peerHeap)

	for i, p := range peerHeap.peers {
		peerHeap.lookupMap[p.ID] = i
	}

	return peerHeap
}

// Put puts peer into heap
// Case 1: Appends a peer if it doesn't exist
// Case 2: Update peer info and reorder if it exists already
func (h *peerHeap) Put(peer *Peer) {
	h.RWMutex.Lock()
	defer h.RWMutex.Unlock()

	index, ok := h.lookupMap[peer.ID]
	if ok {
		// exists already
		h.peers[index] = peer
		heap.Fix(h, index)
	} else {
		heap.Push(h, peer)
	}
}

// BestPeer returns the top of heap
func (h *peerHeap) BestPeer() *Peer {
	h.RWMutex.RLock()
	defer h.RWMutex.RUnlock()

	return h.peers[0]
}

// Remove removes a peer from heap if it exists
func (h *peerHeap) Remove(id string) {
	h.RWMutex.Lock()
	defer h.RWMutex.Unlock()

	index, ok := h.lookupMap[id]
	if ok {
		heap.Remove(h, index)
	}
}

func (h peerHeap) Len() int {
	return len(h.peers)
}

// Less compares the priorities of two items at the passed in indexes (A < B)
func (h peerHeap) Less(i, j int) bool {
	pi, pj := h.peers[i], h.peers[j]

	// sort by number
	if pi.Number != pj.Number {
		// reverse operator because go heap is min heap as default
		return pi.Number > pj.Number
	}

	return pi.Distance < pj.Distance
}

// Swap swaps the places of the items at the passed-in indexes
func (m peerHeap) Swap(i, j int) {
	iid, jid := m.peers[i].ID, m.peers[j].ID
	m.lookupMap[iid] = j
	m.lookupMap[jid] = i

	m.peers[i], m.peers[j] = m.peers[j], m.peers[i]
}

// Push adds a new item to the queue
func (m *peerHeap) Push(x interface{}) {
	peer, ok := x.(*Peer)
	if !ok {
		return
	}

	m.lookupMap[peer.ID] = len(m.peers)
	m.peers = append(m.peers, peer)
}

// Pop removes an item from the queue
func (m *peerHeap) Pop() interface{} {
	old := m.peers
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	m.peers = old[0 : n-1]

	delete(m.lookupMap, item.ID)

	return item
}
