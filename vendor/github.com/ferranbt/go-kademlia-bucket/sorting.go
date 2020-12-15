package kbucket

import (
	"container/list"
	"sort"
)

// A helper struct to sort peers by their distance to the local node
type peerDistance struct {
	p        string
	distance ID
}

// peerDistanceSorter implements sort.Interface to sort peers by xor distance
type peerDistanceSorter struct {
	peers  []peerDistance
	target ID
}

func (pds *peerDistanceSorter) Len() int      { return len(pds.peers) }
func (pds *peerDistanceSorter) Swap(a, b int) { pds.peers[a], pds.peers[b] = pds.peers[b], pds.peers[a] }
func (pds *peerDistanceSorter) Less(a, b int) bool {
	return pds.peers[a].distance.less(pds.peers[b].distance)
}

// Append the string to the sorter's slice. It may no longer be sorted.
func (pds *peerDistanceSorter) appendPeer(entry *Entry) {
	pds.peers = append(pds.peers, peerDistance{
		p:        entry.id,
		distance: xor(pds.target, entry.hash),
	})
}

// Append the string values in the list to the sorter's slice. It may no longer be sorted.
func (pds *peerDistanceSorter) appendPeersFromList(l *list.List) {
	for e := l.Front(); e != nil; e = e.Next() {
		pds.appendPeer(e.Value.(*Entry))
	}
}

func (pds *peerDistanceSorter) sort() {
	sort.Sort(pds)
}
