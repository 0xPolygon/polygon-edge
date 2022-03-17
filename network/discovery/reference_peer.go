package discovery

import (
	"crypto/rand"
	"github.com/libp2p/go-libp2p-core/peer"
	"math/big"
	"sync"
)

// referencePeer keeps track of a single peer and their connection stream
type referencePeer struct {
	id     peer.ID
	stream interface{}
}

// referencePeers keeps track of all the connected peers
type referencePeers struct {
	mux sync.RWMutex

	peersMap map[peer.ID]*referencePeer
}

// isReferencePeer attempts to find a peer among the reference peers
func (rf *referencePeers) isReferencePeer(id peer.ID) *referencePeer {
	rf.mux.RLock()
	defer rf.mux.RUnlock()

	return rf.peersMap[id]
}

// getRandomPeer grabs a random reference peer, if any.
// It's fine to iterate over the map because it's not a common operation
// as peer additions / deletions (where speed actually matters)
func (rf *referencePeers) getRandomPeer() *referencePeer {
	rf.mux.RLock()
	defer rf.mux.RUnlock()

	if len(rf.peersMap) < 1 {
		return nil
	}

	randNum, _ := rand.Int(
		rand.Reader,
		big.NewInt(int64(len(rf.peersMap))),
	)
	randomPeerIndx := int(randNum.Int64())

	counter := 0
	for _, refPeer := range rf.peersMap {
		if randomPeerIndx == counter {
			return refPeer
		}

		counter++
	}

	return nil
}

// addPeer adds a new reference peer if it's not present
func (rf *referencePeers) addPeer(id peer.ID) {
	rf.mux.Lock()
	defer rf.mux.Unlock()

	if _, found := rf.peersMap[id]; found {
		// There is no need to override the peer entry
		// and risk losing the reference to the stream.
		// If the reference to the stream is lost, then it needs
		// to be instantiated again
		return
	}

	rf.peersMap[id] = &referencePeer{
		id:     id,
		stream: nil,
	}
}

// deletePeer deletes a reference peer
func (rf *referencePeers) deletePeer(id peer.ID) {
	rf.mux.Lock()
	defer rf.mux.Unlock()

	delete(rf.peersMap, id)
}
