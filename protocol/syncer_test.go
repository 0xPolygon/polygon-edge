package protocol

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-sdk/blockchain"
	"github.com/0xPolygon/polygon-sdk/helper/tests"
	"github.com/0xPolygon/polygon-sdk/network"
	"github.com/0xPolygon/polygon-sdk/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
)

type mockBlockStore struct {
	blocks       []*types.Block
	subscription *blockchain.MockSubscription
	td           *big.Int
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

	for _, b := range blocks {
		m.td.Add(m.td, big.NewInt(int64(b.Header.Difficulty)))
		m.blocks = append(m.blocks, b)
	}
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
func createNetworkServers(count int, t *testing.T, conf func(c *network.Config)) []*network.Server {
	networkServers := make([]*network.Server, count)

	for indx := 0; indx < count; indx++ {
		networkServers[indx] = network.CreateServer(t, conf)
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
	return res.(int64), nil
}

func TestSyncer_PeerDisconnected(t *testing.T) {
	conf := func(c *network.Config) {
		c.MaxPeers = 4
		c.NoDiscover = true
	}
	blocks := createGenesisBlock()

	// Create three servers
	servers := createNetworkServers(3, t, conf)

	// Create the block stores
	blockStores := createBlockStores(3)

	for _, blockStore := range blockStores {
		assert.NoError(t, blockStore.WriteBlocks(blocks))
	}

	// Create the syncers
	syncers := createSyncers(3, servers, blockStores)

	// Start the syncers
	for _, syncer := range syncers {
		go syncer.Start()
	}

	network.MultiJoin(
		t,
		servers[0],
		servers[1],
		servers[0],
		servers[2],
		servers[1],
		servers[2],
	)

	// wait until gossip protocol builds the mesh network (https://github.com/libp2p/specs/blob/master/pubsub/gossipsub/gossipsub-v1.0.md)
	waitCtx, cancelWait := context.WithTimeout(context.Background(), time.Second*10)
	defer cancelWait()

	numPeers, err := WaitUntilSyncPeersNumber(waitCtx, syncers[1], 2)
	if err != nil {
		t.Fatalf("Unable to add sync peers, %v", err)
	}
	assert.Equal(t, int64(len(servers)-1), numPeers)

	// Disconnect peer2
	peerToDisconnect := servers[2].AddrInfo().ID
	servers[1].Disconnect(peerToDisconnect, "testing")

	waitCtx, cancelWait = context.WithTimeout(context.Background(), time.Second*10)
	defer cancelWait()
	numPeers, err = WaitUntilSyncPeersNumber(waitCtx, syncers[1], 1)
	if err != nil {
		t.Fatalf("Unable to disconnect sync peers, %v", err)
	}
	assert.Equal(t, int64(len(servers)-2), numPeers)

	// server1 syncer should have disconnected from server2 peer
	_, found := syncers[1].peers.Load(peerToDisconnect)
	assert.False(t, found)
}
