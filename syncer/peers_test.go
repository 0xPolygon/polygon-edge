package syncer

import (
	"math/big"
	"sort"
	"testing"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/assert"
)

func getAllTestPeers() []*NoForkPeer {
	return []*NoForkPeer{
		{
			ID:       peer.ID("A"),
			Number:   10,
			Distance: big.NewInt(1),
		},
		{
			ID:       peer.ID("B"),
			Number:   20,
			Distance: big.NewInt(2),
		},
		{
			ID:       peer.ID("C"),
			Number:   20,
			Distance: big.NewInt(1),
		},
	}
}

func cloneNoForkPeers(peers []*NoForkPeer) []*NoForkPeer {
	clone := make([]*NoForkPeer, len(peers))

	for idx, p := range peers {
		clone[idx] = &NoForkPeer{
			ID:       p.ID,
			Number:   p.Number,
			Distance: new(big.Int).Set(p.Distance),
		}
	}

	return clone
}

func sortNoForkPeers(peers []*NoForkPeer) []*NoForkPeer {
	sort.Slice(peers, func(p, q int) bool {
		if peers[p].Number != peers[q].Number {
			return peers[p].Number > peers[q].Number
		}

		return peers[p].Distance.Cmp(peers[q].Distance) < 0
	})

	return peers
}

func peerMapToPeers(peerMap *PeerMap) []*NoForkPeer {
	res := make([]*NoForkPeer, 0)

	for {
		bestPeer := peerMap.BestPeer(nil)
		if bestPeer == nil {
			break
		}

		res = append(res, bestPeer)

		peerMap.Remove(bestPeer.ID)
	}

	return res
}

func TestConstructor(t *testing.T) {
	t.Parallel()

	peers := getAllTestPeers()
	peerMap := NewPeerMap(peers)
	expected := sortNoForkPeers(
		cloneNoForkPeers(peers),
	)
	actual := peerMapToPeers(peerMap)

	assert.Equal(
		t,
		expected,
		actual,
	)
}

func TestPutPeer(t *testing.T) {
	t.Parallel()

	allPeers := getAllTestPeers()
	initialPeers := allPeers[:1]
	peers := allPeers[1:]

	peerMap := NewPeerMap(initialPeers)

	peerMap.Put(peers...)

	expected := sortNoForkPeers(
		cloneNoForkPeers(append(initialPeers, peers...)),
	)

	actual := peerMapToPeers(peerMap)

	assert.Equal(
		t,
		expected,
		actual,
	)
}

func TestBestPeer(t *testing.T) {
	t.Parallel()

	allPeers := getAllTestPeers()

	tests := []struct {
		name     string
		skipList map[peer.ID]bool
		peers    []*NoForkPeer
		result   *NoForkPeer
	}{
		{
			name:     "should return best peer",
			skipList: nil,
			peers:    allPeers,
			result:   allPeers[2],
		},
		{
			name:     "should return null in case of empty map",
			skipList: nil,
			peers:    nil,
			result:   nil,
		},
		{
			name: "should return the 2nd best peer if the best peer is in skip list",
			skipList: map[peer.ID]bool{
				peer.ID("C"): true,
			},
			peers:  allPeers,
			result: allPeers[1],
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			peerMap := NewPeerMap(test.peers)

			bestPeer := peerMap.BestPeer(test.skipList)

			assert.Equal(t, test.result, bestPeer)
		})
	}
}
