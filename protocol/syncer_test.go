package protocol

import (
	"errors"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-sdk/blockchain"
	"github.com/0xPolygon/polygon-sdk/types"
	"github.com/stretchr/testify/assert"
)

func TestHandleUser(t *testing.T) {
	tests := []struct {
		name       string
		chain      blockchainShim
		peerChains []blockchainShim
	}{
		{
			name:  "should set peer's status",
			chain: NewRandomChain(t, 5),
			peerChains: []blockchainShim{
				NewRandomChain(t, 5),
				NewRandomChain(t, 10),
				NewRandomChain(t, 15),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			syncer, peerSyncers := SetupSyncerNetwork(t, tt.chain, tt.peerChains)

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
		chain                blockchainShim
		peerChains           []blockchainShim
		numDisconnectedPeers int
	}{
		{
			name:  "should not have data in peers for disconnected peer",
			chain: NewRandomChain(t, 5),
			peerChains: []blockchainShim{
				NewRandomChain(t, 5),
				NewRandomChain(t, 10),
				NewRandomChain(t, 15),
			},
			numDisconnectedPeers: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			syncer, peerSyncers := SetupSyncerNetwork(t, tt.chain, tt.peerChains)

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
		name         string
		chain        blockchainShim
		peerChain    blockchainShim
		numNewBlocks int
	}{
		{
			name:         "syncer should receive new block in peer",
			chain:        NewRandomChain(t, 5),
			peerChain:    NewRandomChain(t, 10),
			numNewBlocks: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			syncer, peerSyncers := SetupSyncerNetwork(t, tt.chain, []blockchainShim{tt.peerChain})
			peerSyncer := peerSyncers[0]

			newBlocks := GenerateNewBlocks(t, peerSyncer.blockchain, tt.numNewBlocks)
			for _, newBlock := range newBlocks {
				peerSyncer.Broadcast(newBlock)
			}

			peerInfo := syncer.peers[peerSyncer.server.AddrInfo().ID]
			assert.NotNil(t, peerInfo)

			// Check peer's queue
			assert.Len(t, peerInfo.enqueue, tt.numNewBlocks)
			for _, newBlock := range newBlocks {
				block, ok := TryPopBlock(t, syncer, peerSyncer.server.AddrInfo().ID, 10*time.Second)
				assert.True(t, ok, "syncer should be able to pop new block from peer %s", peerSyncer.server.AddrInfo().ID)
				assert.Equal(t, newBlock, block, "syncer should get the same block peer broadcasted")
			}

			// Check peer's status
			lastBlock := newBlocks[len(newBlocks)-1]
			assert.Equal(t, HeaderToStatus(lastBlock.Header), peerInfo.status)
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
				NewRandomChain(t, 10),
			},
			found:         true,
			bestPeerIndex: 1,
		},
		{
			name:  "should find the first peer if there are multiple candidates",
			chain: NewRandomChain(t, 100),
			peersChain: []blockchainShim{
				NewRandomChain(t, 10),
				NewRandomChain(t, 1000),
				NewRandomChain(t, 1000),
				NewRandomChain(t, 100),
			},
			found:         true,
			bestPeerIndex: 1,
		},
		{
			name:  "shouldn't find if all peer doesn't have longer chain than syncer's chain",
			chain: NewRandomChain(t, 1000),
			peersChain: []blockchainShim{
				NewRandomChain(t, 10),
				NewRandomChain(t, 10),
				NewRandomChain(t, 10),
			},
			found: false,
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
				assert.Equal(t, expectedBestPeer.server.AddrInfo().ID.String(), bestPeer.peer.String())
				assert.Equal(t, expectedBestPeerStatus, bestPeer.status)
			} else {
				assert.Nil(t, bestPeer, "syncer shouldn't find best peer, but found")
			}
		})
	}
}

func TestFindCommonAncestor(t *testing.T) {
	tests := []struct {
		name          string
		syncerHeaders []*types.Header
		peerHeaders   []*types.Header
		// result
		found       bool
		headerIndex int
		forkIndex   int
		err         error
	}{
		{
			name:          "should find common ancestor",
			syncerHeaders: blockchain.NewTestHeaderChainWithSeed(nil, 10, 0),
			peerHeaders:   blockchain.NewTestHeaderChainWithSeed(nil, 20, 0),
			found:         true,
			headerIndex:   9,
			forkIndex:     10,
			err:           nil,
		},
		{
			name:          "should return error if there is no fork",
			syncerHeaders: blockchain.NewTestHeaderChainWithSeed(nil, 11, 0),
			peerHeaders:   blockchain.NewTestHeaderChainWithSeed(nil, 10, 0),
			found:         false,
			err:           errors.New("fork not found"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chain, peerChain := blockchain.NewTestBlockchain(t, tt.syncerHeaders), blockchain.NewTestBlockchain(t, tt.peerHeaders)
			syncer, peerSyncers := SetupSyncerNetwork(t, chain, []blockchainShim{peerChain})
			peerSyncer := peerSyncers[0]

			peer := syncer.peers[peerSyncer.server.AddrInfo().ID]
			assert.NotNil(t, peer)

			header, fork, err := syncer.findCommonAncestor(peer.client, peer.status)
			if tt.found {
				assert.Equal(t, tt.peerHeaders[tt.headerIndex], header)
				assert.Equal(t, tt.peerHeaders[tt.forkIndex], fork)
				assert.Nil(t, err)
			} else {
				assert.Nil(t, header)
				assert.Nil(t, fork)
				assert.Equal(t, tt.err, err)
			}
		})
	}
}

func TestWatchSyncWithPeer(t *testing.T) {
	tests := []struct {
		name         string
		headers      []*types.Header
		peerHeaders  []*types.Header
		numNewBlocks int
		// result
		synced         bool
		expectedHeight uint64
	}{
		{
			name:           "should sync until peer's latest block",
			headers:        blockchain.NewTestHeaderChainWithSeed(nil, 10, 0),
			peerHeaders:    blockchain.NewTestHeaderChainWithSeed(nil, 1, 0),
			numNewBlocks:   15,
			synced:         true,
			expectedHeight: 15,
		},
		{
			name:           "shouldn't sync",
			headers:        blockchain.NewTestHeaderChainWithSeed(nil, 10, 0),
			peerHeaders:    blockchain.NewTestHeaderChainWithSeed(nil, 1, 0),
			numNewBlocks:   9,
			synced:         false,
			expectedHeight: 9,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chain, peerChain := NewMockBlockchain(tt.headers), NewMockBlockchain(tt.peerHeaders)
			syncer, peerSyncers := SetupSyncerNetwork(t, chain, []blockchainShim{peerChain})
			peerSyncer := peerSyncers[0]

			newBlocks := GenerateNewBlocks(t, peerChain, tt.numNewBlocks)
			for _, b := range newBlocks {
				peerSyncer.Broadcast(b)
			}

			peer := syncer.peers[peerSyncer.server.AddrInfo().ID]
			assert.NotNil(t, peer)

			latestBlock := newBlocks[len(newBlocks)-1]
			doneCh := make(chan struct{}, 1)
			go func() {
				syncer.WatchSyncWithPeer(peer, func(b *types.Block) bool {
					// sync until latest block
					return b.Header.Number < latestBlock.Header.Number
				})
				// wait until syncer updates status by latest block
				WaitUntilProcessedAllEvents(t, syncer, 10*time.Second)
				doneCh <- struct{}{}
			}()

			select {
			case <-doneCh:
				assert.True(t, tt.synced, "syncer shouldn't sync any block with peer, but did")
				assert.Equal(t, HeaderToStatus(latestBlock.Header), syncer.status)
				assert.Equal(t, tt.expectedHeight, syncer.status.Number)
				break
			case <-time.After(time.Second * 10):
				assert.False(t, tt.synced, "syncer should sync blocks with peer, but didn't")
				break
			}
		})
	}
}

func TestBulkSyncWithPeer(t *testing.T) {
	tests := []struct {
		name        string
		headers     []*types.Header
		peerHeaders []*types.Header
		// result
		synced bool
		err    error
	}{
		{
			name:        "should sync until peer's latest block",
			headers:     blockchain.NewTestHeaderChainWithSeed(nil, 10, 0),
			peerHeaders: blockchain.NewTestHeaderChainWithSeed(nil, 20, 0),
			synced:      true,
			err:         nil,
		},
		{
			name:        "should sync until peer's latest block",
			headers:     blockchain.NewTestHeaderChainWithSeed(nil, 20, 0),
			peerHeaders: blockchain.NewTestHeaderChainWithSeed(nil, 10, 0),
			synced:      false,
			err:         errors.New("fork not found"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chain, peerChain := NewMockBlockchain(tt.headers), NewMockBlockchain(tt.peerHeaders)
			syncer, peerSyncers := SetupSyncerNetwork(t, chain, []blockchainShim{peerChain})
			peerSyncer := peerSyncers[0]

			peer := syncer.peers[peerSyncer.server.AddrInfo().ID]
			assert.NotNil(t, peer)

			err := syncer.BulkSyncWithPeer(peer)
			assert.Equal(t, tt.err, err)
			WaitUntilProcessedAllEvents(t, syncer, 10*time.Second)

			expectedStatus := HeaderToStatus(tt.headers[len(tt.headers)-1])
			if tt.synced {
				expectedStatus = HeaderToStatus(tt.peerHeaders[len(tt.peerHeaders)-1])
			}
			assert.Equal(t, expectedStatus, syncer.status)
		})
	}
}
