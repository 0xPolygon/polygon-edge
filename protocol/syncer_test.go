package protocol

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestHandleUser(t *testing.T) {
	tests := []struct {
		name       string
		blockchain blockchainShim
		peerChains []blockchainShim
	}{
		{
			name:       "should get peer's status",
			blockchain: NewRandomChainWithRandomHeight(t),
			peerChains: []blockchainShim{
				NewRandomChainWithRandomHeight(t),
				NewRandomChainWithRandomHeight(t),
				NewRandomChainWithRandomHeight(t),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			syncer, peerSyncers := SetupSyncerNetwork(t, tt.blockchain, tt.peerChains)

			// Check peer's status in Syncer's peer list
			for _, peerSyncer := range peerSyncers {
				peer := syncer.peers[peerSyncer.server.AddrInfo().ID]
				assert.NotNil(t, peer, "syncer must have peer's status, but nil")

				// should receive latest status
				expectedStatus := GetCurrentStatus(peerSyncer.blockchain)
				assert.Equal(t, expectedStatus, peer.status)
			}
		})
	}
}

func TestDeleteUser(t *testing.T) {
	tests := []struct {
		name                 string
		blockchain           blockchainShim
		peerChains           []blockchainShim
		numDisconnectedPeers int
	}{
		{
			name:       "should not have data in peers for disconnected peer",
			blockchain: NewRandomChainWithRandomHeight(t),
			peerChains: []blockchainShim{
				NewRandomChainWithRandomHeight(t),
				NewRandomChainWithRandomHeight(t),
				NewRandomChainWithRandomHeight(t),
			},
			numDisconnectedPeers: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			syncer, peerSyncers := SetupSyncerNetwork(t, tt.blockchain, tt.peerChains)

			// disconnects from syncer
			for i := 0; i < tt.numDisconnectedPeers; i++ {
				peerSyncers[i].server.Disconnect(syncer.server.AddrInfo().ID, "bye")
			}
			WaitUntilPeerConnected(t, syncer, len(tt.peerChains)-tt.numDisconnectedPeers, 10*time.Second)

			for idx, peerSyncer := range peerSyncers {
				shouldBeDeleted := idx < tt.numDisconnectedPeers

				peer := syncer.peers[peerSyncer.server.AddrInfo().ID]
				if shouldBeDeleted {
					assert.Nil(t, peer)
				} else {
					assert.NotNil(t, peer)
				}
			}
		})
	}
}

func TestBroadcast(t *testing.T) {
	tests := []struct {
		name          string
		initChain     blockchainShim
		initPeerChain blockchainShim
		numNewBlocks  int
	}{
		{
			name:          "syncer should receive new block in peer",
			initChain:     NewRandomChainWithRandomHeight(t),
			initPeerChain: NewRandomChainWithRandomHeight(t),
			numNewBlocks:  5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			syncer, peerSyncers := SetupSyncerNetwork(t, tt.initChain, []blockchainShim{tt.initPeerChain})
			peerSyncer := peerSyncers[0]

			newBlocks := GenerateNewBlocks(t, peerSyncer.blockchain, tt.numNewBlocks)
			for _, newBlock := range newBlocks {
				peerSyncer.Broadcast(newBlock)
				block, ok := TryPopBlock(t, syncer, peerSyncer.server.AddrInfo().ID, 10*time.Second)
				assert.True(t, ok, "syncer should be able to pop new block from peer %s", peerSyncer.server.AddrInfo().ID)
				assert.Equal(t, newBlock, block, "syncer should get the same block peer broadcasted")
			}

			peerInfo := syncer.peers[peerSyncer.server.AddrInfo().ID]
			assert.NotNil(t, peerInfo)

			// latest peer's status should be based on latest peer's block
			lastBlock := newBlocks[len(newBlocks)-1]
			expectedPeerStatus := BlockToStatus(lastBlock)
			assert.Equal(t, expectedPeerStatus, peerInfo.status)
		})
	}
}

func TestBestPeer(t *testing.T) {
	tests := []struct {
		name          string
		chain         blockchainShim
		peersChain    []blockchainShim
		found         bool
		bestPeerIndex int
	}{
		{
			name:  "should find the peer that has the longest chain",
			chain: NewRandomChain(t, 100),
			peersChain: []blockchainShim{
				NewRandomChain(t, 10),
				NewRandomChain(t, 1000),
				NewRandomChain(t, 100),
			},
			found:         true,
			bestPeerIndex: 1,
		},
		{
			name:  "should find the first peer if there are multiple candidates",
			chain: NewRandomChain(t, 100),
			peersChain: []blockchainShim{
				NewRandomChain(t, 1000),
				NewRandomChain(t, 1000),
				NewRandomChain(t, 100),
			},
			found:         true,
			bestPeerIndex: 0,
		},
		{
			name:  "shouldn't find if all peer doesn't have longer chain than syncer's chain",
			chain: NewRandomChain(t, 1000),
			peersChain: []blockchainShim{
				NewRandomChain(t, 10),
				NewRandomChain(t, 10),
				NewRandomChain(t, 10),
			},
			found:         false,
			bestPeerIndex: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			syncer, peerSyncers := SetupSyncerNetwork(t, tt.chain, tt.peersChain)

			bestPeer := syncer.BestPeer()
			if tt.found {
				assert.NotNil(t, bestPeer, "syncer should find best peer, but not found")

				expectedBestPeer := peerSyncers[tt.bestPeerIndex]
				expectedBestPeerStatus := GetCurrentStatus(expectedBestPeer.blockchain)
				assert.Equal(t, expectedBestPeer.server.AddrInfo().ID, bestPeer.peer)
				assert.Equal(t, expectedBestPeerStatus, bestPeer.status)
			} else {
				assert.Nil(t, bestPeer, "syncer shouldn't find best peer, but found")
			}
		})
	}
}
