package protocol

import (
	"math/big"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-sdk/blockchain"
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
func TestSyncer_PeerDisconnected(t *testing.T) {
	conf := func(c *network.Config) {
		c.MaxPeers = 4
		c.NoDiscover = true
	}
	blocks := createGenesisBlock()
	// Create three servers
	srv0 := network.CreateServer(t, conf)
	srv1 := network.CreateServer(t, conf)
	srv2 := network.CreateServer(t, conf)

	bstores := createBlockStores(3)

	bstores[0].WriteBlocks(blocks)
	bstores[1].WriteBlocks(blocks)
	bstores[2].WriteBlocks(blocks)

	sync0 := NewSyncer(hclog.NewNullLogger(), srv0, bstores[0])
	sync1 := NewSyncer(hclog.NewNullLogger(), srv1, bstores[1])
	sync2 := NewSyncer(hclog.NewNullLogger(), srv2, bstores[2])

	go sync0.Start()
	go sync1.Start()
	go sync2.Start()

	network.MultiJoin(t, srv0, srv1, srv0, srv2, srv1, srv2)
	time.Sleep(1 * time.Second)
	//Disconnect peer peer2
	srv1.Disconnect(srv2.AddrInfo().ID, "testing")
	time.Sleep(1 * time.Second)

	assert.NotContains(t, sync1.peers, srv2.AddrInfo().ID)

}
