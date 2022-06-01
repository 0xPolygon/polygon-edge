package protocol

import (
	"context"
	"crypto/rand"
	"math"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/blockchain"
	"github.com/0xPolygon/polygon-edge/helper/tests"
	"github.com/0xPolygon/polygon-edge/network"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/stretchr/testify/assert"
)

const (
	maxSeed = math.MaxInt32
)

var (
	defaultNetworkConfig = func(c *network.Config) {
		c.NoDiscover = true
	}
)

// getPeer returns a peer with given ID in syncer's map
func getPeer(syncer *Syncer, id peer.ID) *SyncPeer {
	rawPeer, ok := syncer.peers.Load(id)
	if !ok {
		return nil
	}

	syncPeer, ok := rawPeer.(*SyncPeer)
	if !ok {
		return nil
	}

	return syncPeer
}

// CreateSyncer initialize syncer with server
func CreateSyncer(t *testing.T, blockchain blockchainShim, serverCfg *func(c *network.Config)) *Syncer {
	t.Helper()

	if serverCfg == nil {
		serverCfg = &defaultNetworkConfig
	}

	srv, createErr := network.CreateServer(&network.CreateServerParams{
		ConfigCallback: *serverCfg,
	})
	if createErr != nil {
		t.Fatalf("Unable to create networking server, %v", createErr)
	}

	syncer := NewSyncer(hclog.NewNullLogger(), srv, blockchain)
	syncer.Start()

	return syncer
}

// WaitUntilPeerConnected waits until syncer connects to given number of peers
func WaitUntilPeerConnected(t *testing.T, syncer *Syncer, numPeer int, timeout time.Duration) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)

	t.Cleanup(func() {
		cancel()
	})

	countMap := func(m *sync.Map) int {
		count := 0

		m.Range(func(key, value interface{}) bool {
			count++

			return true
		})

		return count
	}

	_, err := tests.RetryUntilTimeout(ctx, func() (interface{}, bool) {
		num := countMap(&syncer.peers)
		if num == numPeer {
			return nil, false
		}

		return nil, true
	})
	assert.NoError(t, err)
}

// WaitUntilProcessedAllEvents waits until syncer finish to process all blockchain events
func WaitUntilProcessedAllEvents(t *testing.T, syncer *Syncer, timeout time.Duration) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)

	t.Cleanup(func() {
		cancel()
	})

	_, err := tests.RetryUntilTimeout(ctx, func() (interface{}, bool) {
		return nil, len(syncer.blockchain.SubscribeEvents().GetEventCh()) > 0
	})
	assert.NoError(t, err)
}

// WaitUntilProgressionUpdated waits until the syncer's progression current block reaches a target
func WaitUntilProgressionUpdated(t *testing.T, syncer *Syncer, timeout time.Duration, target uint64) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)

	t.Cleanup(func() {
		cancel()
	})

	_, err := tests.RetryUntilTimeout(ctx, func() (interface{}, bool) {
		return nil, syncer.syncProgression.GetProgression().CurrentBlock < target
	})
	assert.NoError(t, err)
}

// NewRandomChain returns new blockchain with random seed
func NewRandomChain(t *testing.T, height int) blockchainShim {
	t.Helper()

	randNum, _ := rand.Int(rand.Reader, big.NewInt(int64(maxSeed)))

	return blockchain.NewTestBlockchain(
		t,
		blockchain.NewTestHeadersWithSeed(
			nil,
			height,
			randNum.Uint64(),
		),
	)
}

// SetupSyncerNetwork connects syncers
func SetupSyncerNetwork(
	t *testing.T,
	chain blockchainShim,
	peerChains []blockchainShim,
) (syncer *Syncer, peerSyncers []*Syncer) {
	t.Helper()

	syncer = CreateSyncer(t, chain, nil)
	peerSyncers = make([]*Syncer, len(peerChains))

	for idx, peerChain := range peerChains {
		peerSyncers[idx] = CreateSyncer(t, peerChain, nil)

		if joinErr := network.JoinAndWait(
			syncer.server,
			peerSyncers[idx].server,
			network.DefaultBufferTimeout,
			network.DefaultJoinTimeout,
		); joinErr != nil {
			t.Fatalf("Unable to join servers, %v", joinErr)
		}
	}

	return syncer, peerSyncers
}

// GenerateNewBlocks returns new blocks from latest block of given chain
func GenerateNewBlocks(t *testing.T, chain blockchainShim, num int) []*types.Block {
	t.Helper()

	currentHeight := chain.Header().Number
	oldHeaders := make([]*types.Header, currentHeight+1)

	for i := uint64(1); i <= currentHeight; i++ {
		var ok bool
		oldHeaders[i], ok = chain.GetHeaderByNumber(i)
		assert.Truef(t, ok, "chain should have header at %d, but empty", i)
	}

	headers := blockchain.AppendNewTestHeaders(oldHeaders, num)

	return blockchain.HeadersToBlocks(headers[currentHeight+1:])
}

// TryPopBlock tries to take block from peer's queue in syncer within timeout
func TryPopBlock(t *testing.T, syncer *Syncer, peerID peer.ID, timeout time.Duration) (*types.Block, bool) {
	t.Helper()

	peer := getPeer(syncer, peerID)
	assert.NotNil(t, peer, "syncer doesn't have peer %s", peerID.String())

	blockCh := make(chan *types.Block, 1)

	go func() {
		if block, _ := peer.popBlock(popTimeout); block != nil {
			blockCh <- block
		}
	}()

	select {
	case block := <-blockCh:
		return block, true
	case <-time.After(timeout):
		return nil, false
	}
}

// GetCurrentStatus return status by latest block in blockchain
func GetCurrentStatus(b blockchainShim) *Status {
	return &Status{
		Hash:       b.Header().Hash,
		Number:     b.Header().Number,
		Difficulty: b.CurrentTD(),
	}
}

// HeaderToStatus converts given header to Status
func HeaderToStatus(h *types.Header) *Status {
	var td uint64 = 0
	for i := uint64(1); i <= h.Difficulty; i++ {
		td = td + i
	}

	return &Status{
		Hash:       h.Hash,
		Number:     h.Number,
		Difficulty: big.NewInt(0).SetUint64(td),
	}
}

// mockBlockchain is a mock of blockhain for syncer tests
type mockBlockchain struct {
	blocks        []*types.Block
	subscriptions []*mockSubscription
}

func (b *mockBlockchain) CalculateGasLimit(number uint64) (uint64, error) {
	panic("implement me")
}

func NewMockBlockchain(headers []*types.Header) *mockBlockchain {
	return &mockBlockchain{
		blocks:        blockchain.HeadersToBlocks(headers),
		subscriptions: make([]*mockSubscription, 0),
	}
}

func (b *mockBlockchain) SubscribeEvents() blockchain.Subscription {
	subscription := NewMockSubscription()
	b.subscriptions = append(b.subscriptions, subscription)

	return subscription
}

func (b *mockBlockchain) Header() *types.Header {
	l := len(b.blocks)
	if l == 0 {
		return nil
	}

	return b.blocks[l-1].Header
}

func (b *mockBlockchain) CurrentTD() *big.Int {
	current := b.Header()
	if current == nil {
		return nil
	}

	td, _ := b.GetTD(current.Hash)

	return td
}

func (b *mockBlockchain) GetTD(hash types.Hash) (*big.Int, bool) {
	var td uint64 = 0
	for _, b := range b.blocks {
		td = td + b.Header.Difficulty

		if b.Header.Hash == hash {
			return big.NewInt(0).SetUint64(td), true
		}
	}

	return nil, false
}

func (b *mockBlockchain) GetReceiptsByHash(types.Hash) ([]*types.Receipt, error) {
	panic("not implement")
}

func (b *mockBlockchain) GetBodyByHash(types.Hash) (*types.Body, bool) {
	return &types.Body{}, true
}

func (b *mockBlockchain) GetHeaderByHash(h types.Hash) (*types.Header, bool) {
	for _, b := range b.blocks {
		if b.Header.Hash == h {
			return b.Header, true
		}
	}

	return nil, false
}

func (b *mockBlockchain) GetHeaderByNumber(n uint64) (*types.Header, bool) {
	for _, b := range b.blocks {
		if b.Header.Number == n {
			return b.Header, true
		}
	}

	return nil, false
}

func (b *mockBlockchain) WriteBlock(block *types.Block) error {
	b.blocks = append(b.blocks, block)
	for _, subscription := range b.subscriptions {
		subscription.AppendBlock(block)
	}

	return nil
}

func (b *mockBlockchain) VerifyFinalizedBlock(block *types.Block) error {
	return nil
}

func (b *mockBlockchain) WriteBlocks(blocks []*types.Block) error {
	for _, block := range blocks {
		if writeErr := b.WriteBlock(block); writeErr != nil {
			return writeErr
		}
	}

	return nil
}

// mockSubscription is a mock of subscription for blockchain events
type mockSubscription struct {
	eventCh chan *blockchain.Event
}

func NewMockSubscription() *mockSubscription {
	return &mockSubscription{
		eventCh: make(chan *blockchain.Event),
	}
}

func (s *mockSubscription) AppendBlock(block *types.Block) {
	s.eventCh <- &blockchain.Event{
		NewChain:   []*types.Header{block.Header},
		Difficulty: HeaderToStatus(block.Header).Difficulty,
	}
}

func (s *mockSubscription) GetEventCh() chan *blockchain.Event {
	return s.eventCh
}

func (s *mockSubscription) GetEvent() *blockchain.Event {
	return <-s.eventCh
}

func (s *mockSubscription) Close() {
	close(s.eventCh)
}
