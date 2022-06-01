package protocol

import (
	"context"
	"errors"
	"github.com/0xPolygon/polygon-edge/protocol/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"math/big"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/blockchain"
	"github.com/0xPolygon/polygon-edge/helper/tests"
	"github.com/0xPolygon/polygon-edge/network"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
)

// !! [Used only for testing] !!
// SyncBroadcast broadcasts a block to all peers [synchronous]/
func (s *Syncer) SyncBroadcast(b *types.Block) {
	// Get the chain difficulty associated with block
	td, ok := s.blockchain.GetTD(b.Hash())
	if !ok {
		// not supposed to happen
		s.logger.Error("total difficulty not found", "block number", b.Number())

		return
	}

	// broadcast the new block to all the peers
	req := &proto.NotifyReq{
		Status: &proto.V1Status{
			Hash:       b.Hash().String(),
			Number:     b.Number(),
			Difficulty: td.String(),
		},
		Raw: &anypb.Any{
			Value: b.MarshalRLP(),
		},
	}

	//	notify peers in the background
	s.notifyPeers(req)
}

func TestHandleNewPeer(t *testing.T) {
	t.Parallel()

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
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			syncer, peerSyncers := SetupSyncerNetwork(t, tt.chain, tt.peerChains)

			// Check peer's status in Syncer's peer list
			for _, peerSyncer := range peerSyncers {
				peer := getPeer(syncer, peerSyncer.server.AddrInfo().ID)
				assert.NotNil(t, peer, "syncer must have peer's status, but nil")

				// should receive latest status
				expectedStatus := GetCurrentStatus(peerSyncer.blockchain)
				assert.Equal(t, expectedStatus, peer.status)
			}
		})
	}
}

func TestDeletePeer(t *testing.T) {
	t.Parallel()

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
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			syncer, peerSyncers := SetupSyncerNetwork(t, tt.chain, tt.peerChains)

			// disconnects from syncer
			for i := 0; i < tt.numDisconnectedPeers; i++ {
				peerSyncers[i].server.DisconnectFromPeer(syncer.server.AddrInfo().ID, "bye")
			}
			WaitUntilPeerConnected(t, syncer, len(tt.peerChains)-tt.numDisconnectedPeers, 10*time.Second)

			for idx, peerSyncer := range peerSyncers {
				shouldBeDeleted := idx < tt.numDisconnectedPeers
				peer := getPeer(syncer, peerSyncer.server.AddrInfo().ID)
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
	t.Parallel()

	tests := []struct {
		name          string
		syncerHeaders []*types.Header
		peerHeaders   []*types.Header
		numNewBlocks  int
	}{
		{
			name:          "syncer should receive new block in peer",
			syncerHeaders: blockchain.NewTestHeadersWithSeed(nil, 5, 0),
			peerHeaders:   blockchain.NewTestHeadersWithSeed(nil, 10, 0),
			numNewBlocks:  5,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			chain, peerChain := NewMockBlockchain(tt.syncerHeaders), NewMockBlockchain(tt.peerHeaders)
			syncer, peerSyncers := SetupSyncerNetwork(t, chain, []blockchainShim{peerChain})
			peerSyncer := peerSyncers[0]

			newBlocks := GenerateNewBlocks(t, peerSyncer.blockchain, tt.numNewBlocks)

			for _, newBlock := range newBlocks {
				assert.NoError(t, peerSyncer.blockchain.VerifyFinalizedBlock(newBlock))
				assert.NoError(t, peerSyncer.blockchain.WriteBlock(newBlock))
			}

			for _, newBlock := range newBlocks {
				peerSyncer.SyncBroadcast(newBlock)
			}

			peer := getPeer(syncer, peerSyncer.server.AddrInfo().ID)
			assert.NotNil(t, peer)

			// Check peer's queue
			assert.Len(t, peer.enqueue, tt.numNewBlocks)
			for _, newBlock := range newBlocks {
				block, ok := TryPopBlock(t, syncer, peerSyncer.server.AddrInfo().ID, 10*time.Second)
				assert.True(t, ok, "syncer should be able to pop new block from peer %s", peerSyncer.server.AddrInfo().ID)
				assert.Equal(t, newBlock, block, "syncer should get the same block peer broadcasted")
			}

			// Check peer's status
			lastBlock := newBlocks[len(newBlocks)-1]
			assert.Equal(t, HeaderToStatus(lastBlock.Header), peer.status)
		})
	}
}

func TestBestPeer(t *testing.T) {
	t.Parallel()

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
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

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

func TestWatchSyncWithPeer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		headers        []*types.Header
		peerHeaders    []*types.Header
		numNewBlocks   int
		shouldSync     bool
		expectedHeight uint64
	}{
		{
			name:           "should sync until peer's latest block",
			headers:        blockchain.NewTestHeadersWithSeed(nil, 10, 0),
			peerHeaders:    blockchain.NewTestHeadersWithSeed(nil, 1, 0),
			numNewBlocks:   15,
			shouldSync:     true,
			expectedHeight: 15,
		},
		{
			name:           "shouldn't sync",
			headers:        blockchain.NewTestHeadersWithSeed(nil, 10, 0),
			peerHeaders:    blockchain.NewTestHeadersWithSeed(nil, 1, 0),
			numNewBlocks:   9,
			shouldSync:     false,
			expectedHeight: 9,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			chain, peerChain := NewMockBlockchain(tt.headers), NewMockBlockchain(tt.peerHeaders)

			syncer, peerSyncers := SetupSyncerNetwork(t, chain, []blockchainShim{peerChain})
			peerSyncer := peerSyncers[0]

			newBlocks := GenerateNewBlocks(t, peerChain, tt.numNewBlocks)

			for _, newBlock := range newBlocks {
				assert.NoError(t, peerSyncer.blockchain.VerifyFinalizedBlock(newBlock))
				assert.NoError(t, peerSyncer.blockchain.WriteBlock(newBlock))
			}

			for _, b := range newBlocks {
				peerSyncer.SyncBroadcast(b)
			}

			peer := getPeer(syncer, peerSyncer.server.AddrInfo().ID)
			assert.NotNil(t, peer)

			latestBlock := newBlocks[len(newBlocks)-1]
			startSyncTime := time.Now()
			endSyncTime := startSyncTime.Add(time.Second * 5)
			syncer.WatchSyncWithPeer(peer, func(b *types.Block) bool {
				if time.Now().After(endSyncTime) {
					// Timeout
					return true
				}
				// sync until latest block
				return b.Header.Number >= latestBlock.Header.Number
			}, 2)

			if tt.shouldSync {
				assert.Equal(t, HeaderToStatus(latestBlock.Header), syncer.status)
			}

			assert.Equal(t, tt.expectedHeight, syncer.status.Number)
		})
	}
}

func TestBulkSyncWithPeer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		headers     []*types.Header
		peerHeaders []*types.Header
		// result
		shouldSync    bool
		syncFromBlock int
	}{
		{
			name:          "should sync until peer's latest block",
			headers:       blockchain.NewTestHeadersWithSeed(nil, 10, 0),
			peerHeaders:   blockchain.NewTestHeadersWithSeed(nil, 30, 0),
			shouldSync:    true,
			syncFromBlock: 10,
		},
		{
			name:          "shouldn't sync if peer's latest block is behind",
			headers:       blockchain.NewTestHeadersWithSeed(nil, 20, 0),
			peerHeaders:   blockchain.NewTestHeadersWithSeed(nil, 10, 0),
			shouldSync:    false,
			syncFromBlock: 0,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			chain, peerChain := NewMockBlockchain(tt.headers), NewMockBlockchain(tt.peerHeaders)
			syncer, peerSyncers := SetupSyncerNetwork(t, chain, []blockchainShim{peerChain})
			peerSyncer := peerSyncers[0]
			var handledNewBlocks []*types.Block
			newBlocksHandler := func(block *types.Block) {
				handledNewBlocks = append(handledNewBlocks, block)
			}

			peer := getPeer(syncer, peerSyncer.server.AddrInfo().ID)
			assert.NotNil(t, peer)

			err := syncer.BulkSyncWithPeer(peer, newBlocksHandler)
			assert.NoError(t, err)
			WaitUntilProcessedAllEvents(t, syncer, 10*time.Second)

			var expectedStatus *Status
			if tt.shouldSync {
				expectedStatus = HeaderToStatus(tt.peerHeaders[len(tt.peerHeaders)-1])
				assert.Equal(t, handledNewBlocks, peerChain.blocks[tt.syncFromBlock:], "not all blocks are handled")
				assert.Equal(t, peerChain.blocks, chain.blocks, "chain is not synced")
			} else {
				expectedStatus = HeaderToStatus(tt.headers[len(tt.headers)-1])
				assert.NotEqual(t, handledNewBlocks, peerChain.blocks[tt.syncFromBlock:])
				assert.NotEqual(t, peerChain.blocks, chain.blocks)
			}
			assert.Equal(t, expectedStatus, syncer.status)
		})
	}
}

func TestSyncer_GetSyncProgression(t *testing.T) {
	initialChainSize := 10
	targetChainSize := 1000

	existingChain := blockchain.NewTestHeadersWithSeed(nil, initialChainSize, 0)
	syncerChain := NewMockBlockchain(existingChain)
	syncer := CreateSyncer(t, syncerChain, nil)

	syncHeaders := blockchain.NewTestHeadersWithSeed(nil, targetChainSize, 0)
	syncBlocks := blockchain.HeadersToBlocks(syncHeaders)

	syncer.syncProgression.StartProgression(uint64(initialChainSize), syncerChain.SubscribeEvents())

	if syncer.GetSyncProgression() == nil {
		t.Fatalf("Unable to start progression")
	}

	assert.Equal(t, uint64(initialChainSize), syncer.syncProgression.GetProgression().StartingBlock)

	syncer.syncProgression.UpdateHighestProgression(uint64(targetChainSize))

	assert.Equal(t, uint64(targetChainSize), syncer.syncProgression.GetProgression().HighestBlock)

	writeErr := syncerChain.WriteBlocks(syncBlocks[initialChainSize+1:])

	assert.NoError(t, writeErr)

	WaitUntilProgressionUpdated(t, syncer, 15*time.Second, uint64(targetChainSize-1))
	assert.Equal(t, uint64(targetChainSize-1), syncer.syncProgression.GetProgression().CurrentBlock)

	syncer.syncProgression.StopProgression()
}

type mockBlockStore struct {
	blocks       []*types.Block
	subscription *blockchain.MockSubscription
	td           *big.Int
}

func (m *mockBlockStore) CalculateGasLimit(number uint64) (uint64, error) {
	panic("implement me")
}

func newMockBlockStore() *mockBlockStore {
	bs := &mockBlockStore{
		blocks:       make([]*types.Block, 0),
		subscription: blockchain.NewMockSubscription(),
		td:           big.NewInt(1),
	}

	return bs
}

func (m *mockBlockStore) Header() *types.Header {
	return m.blocks[len(m.blocks)-1].Header
}
func (m *mockBlockStore) GetHeaderByNumber(n uint64) (*types.Header, bool) {
	b, ok := m.GetBlockByNumber(n, false)
	if !ok {
		return nil, false
	}

	return b.Header, true
}
func (m *mockBlockStore) GetBlockByNumber(blockNumber uint64, full bool) (*types.Block, bool) {
	for _, b := range m.blocks {
		if b.Number() == blockNumber {
			return b, true
		}
	}

	return nil, false
}
func (m *mockBlockStore) SubscribeEvents() blockchain.Subscription {
	return m.subscription
}
func (m *mockBlockStore) GetReceiptsByHash(types.Hash) ([]*types.Receipt, error) {
	return nil, nil
}

func (m *mockBlockStore) GetHeaderByHash(hash types.Hash) (*types.Header, bool) {
	for _, b := range m.blocks {
		header := b.Header.ComputeHash()
		if header.Hash == hash {
			return header, true
		}
	}

	return nil, true
}
func (m *mockBlockStore) GetBodyByHash(hash types.Hash) (*types.Body, bool) {
	for _, b := range m.blocks {
		if b.Hash() == hash {
			return b.Body(), true
		}
	}

	return nil, true
}

func (m *mockBlockStore) WriteBlocks(blocks []*types.Block) error {
	for _, block := range blocks {
		if writeErr := m.WriteBlock(block); writeErr != nil {
			return writeErr
		}
	}

	return nil
}

func (m *mockBlockStore) VerifyFinalizedBlock(block *types.Block) error {
	return nil
}

func (m *mockBlockStore) WriteBlock(block *types.Block) error {
	m.td.Add(m.td, big.NewInt(int64(block.Header.Difficulty)))
	m.blocks = append(m.blocks, block)

	return nil
}

func (m *mockBlockStore) CurrentTD() *big.Int {
	return m.td
}

func (m *mockBlockStore) GetTD(hash types.Hash) (*big.Int, bool) {
	return m.td, false
}

func createGenesisBlock() []*types.Block {
	blocks := make([]*types.Block, 0)
	genesis := &types.Header{Difficulty: 1, Number: 0}
	genesis.ComputeHash()

	b := &types.Block{
		Header: genesis,
	}
	blocks = append(blocks, b)

	return blocks
}

func createBlockStores(count int) (bStore []*mockBlockStore) {
	bStore = make([]*mockBlockStore, count)
	for i := 0; i < count; i++ {
		bStore[i] = newMockBlockStore()
	}

	return
}

// createNetworkServers is a helper function for generating network servers
func createNetworkServers(t *testing.T, count int, conf func(c *network.Config)) []*network.Server {
	t.Helper()

	networkServers := make([]*network.Server, count)

	for indx := 0; indx < count; indx++ {
		server, createErr := network.CreateServer(&network.CreateServerParams{ConfigCallback: conf})
		if createErr != nil {
			t.Fatalf("Unable to create network servers, %v", createErr)
		}

		networkServers[indx] = server
	}

	return networkServers
}

// createSyncers is a helper function for generating syncers. Servers and BlockStores should be at least the length
// of count
func createSyncers(count int, servers []*network.Server, blockStores []*mockBlockStore) []*Syncer {
	syncers := make([]*Syncer, count)

	for indx := 0; indx < count; indx++ {
		syncers[indx] = NewSyncer(hclog.NewNullLogger(), servers[indx], blockStores[indx])
	}

	return syncers
}

// numSyncPeers returns the number of sync peers
func numSyncPeers(syncer *Syncer) int64 {
	num := 0

	syncer.peers.Range(func(key, value interface{}) bool {
		num++

		return true
	})

	return int64(num)
}

// WaitUntilSyncPeersNumber waits until the number of sync peers reaches a certain number, otherwise it times out
func WaitUntilSyncPeersNumber(ctx context.Context, syncer *Syncer, requiredNum int64) (int64, error) {
	res, err := tests.RetryUntilTimeout(ctx, func() (interface{}, bool) {
		numPeers := numSyncPeers(syncer)
		if numPeers == requiredNum {
			return numPeers, false
		}

		return nil, true
	})

	if err != nil {
		return 0, err
	}

	resVal, ok := res.(int64)
	if !ok {
		return 0, errors.New("invalid type assert")
	}

	return resVal, nil
}

func TestSyncer_PeerDisconnected(t *testing.T) {
	conf := func(c *network.Config) {
		c.MaxInboundPeers = 4
		c.MaxOutboundPeers = 4
		c.NoDiscover = true
	}
	blocks := createGenesisBlock()

	// Create three servers
	servers := createNetworkServers(t, 3, conf)

	// Create the block stores
	blockStores := createBlockStores(3)

	for _, blockStore := range blockStores {
		assert.NoError(t, blockStore.WriteBlocks(blocks))
	}

	// Create the syncers
	syncers := createSyncers(3, servers, blockStores)

	// Start the syncers
	for _, syncer := range syncers {
		syncer.Start()
	}

	joinErrors := network.MeshJoin(servers...)
	if len(joinErrors) != 0 {
		t.Fatalf("Unable to join servers [%d], %v", len(joinErrors), joinErrors)
	}

	// wait until gossip protocol builds the mesh network
	// (https://github.com/libp2p/specs/blob/master/pubsub/gossipsub/gossipsub-v1.0.md)
	waitCtx, cancelWait := context.WithTimeout(context.Background(), time.Second*10)
	defer cancelWait()

	numPeers, err := WaitUntilSyncPeersNumber(waitCtx, syncers[1], 2)
	if err != nil {
		t.Fatalf("Unable to add sync peers, %v", err)
	}
	// Make sure the number of peers is correct
	// -1 to exclude the current node
	assert.Equal(t, int64(len(servers)-1), numPeers)

	// Disconnect peer2
	peerToDisconnect := servers[2].AddrInfo().ID
	servers[1].DisconnectFromPeer(peerToDisconnect, "testing")

	waitCtx, cancelWait = context.WithTimeout(context.Background(), time.Second*10)
	defer cancelWait()

	numPeers, err = WaitUntilSyncPeersNumber(waitCtx, syncers[1], 1)

	if err != nil {
		t.Fatalf("Unable to disconnect sync peers, %v", err)
	}
	// Make sure a single peer disconnected
	// Additional -1 to exclude the current node
	assert.Equal(t, int64(len(servers)-2), numPeers)

	// server1 syncer should have disconnected from server2 peer
	_, found := syncers[1].peers.Load(peerToDisconnect)

	// Make sure that the disconnected peer is not in the
	// reference node's sync peer map
	assert.False(t, found)
}

func TestVerifyNotifyRequest(t *testing.T) {
	t.Parallel()

	testTable := []struct {
		name        string
		request     *proto.NotifyReq
		expectedErr error
	}{
		{
			"Valid notify request",
			&proto.NotifyReq{
				Status: &proto.V1Status{},
				Raw:    &anypb.Any{},
			},
			nil,
		},
		{
			"No notify request",
			nil,
			errMalformedNotifyRequest,
		},
		{
			"No notify body",
			&proto.NotifyReq{
				Status: &proto.V1Status{},
				Raw:    nil,
			},
			errMalformedNotifyBody,
		},
		{
			"No notify status",
			&proto.NotifyReq{
				Status: nil,
				Raw:    &anypb.Any{},
			},
			errMalformedNotifyStatus,
		},
	}

	for _, testCase := range testTable {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			assert.ErrorIs(
				t,
				verifyNotifyRequest(testCase.request),
				testCase.expectedErr,
			)
		})
	}
}

func TestVerifyHeadersResponse(t *testing.T) {
	t.Parallel()

	testTable := []struct {
		name        string
		response    *proto.Response_Component
		expectedErr error
	}{
		{
			"Valid headers response",
			&proto.Response_Component{
				Spec: &anypb.Any{},
			},
			nil,
		},
		{
			"No headers response",
			nil,
			errMalformedHeadersResponse,
		},
		{
			"No headers data",
			&proto.Response_Component{
				Spec: nil,
			},
			errMalformedHeadersBody,
		},
	}

	for _, testCase := range testTable {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			assert.ErrorIs(
				t,
				verifyHeadersResponse(testCase.response),
				testCase.expectedErr,
			)
		})
	}
}
