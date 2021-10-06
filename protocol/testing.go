package protocol

import (
	"context"
	"math"
	"math/big"
	"math/rand"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-sdk/blockchain"
	"github.com/0xPolygon/polygon-sdk/helper/tests"
	"github.com/0xPolygon/polygon-sdk/network"
	"github.com/0xPolygon/polygon-sdk/types"
	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/stretchr/testify/assert"
)

const (
	maxHeight = 1024
	maxSeed   = math.MaxInt32
)

var (
	defaultNetworkConfig = func(c *network.Config) {
		c.NoDiscover = true
	}
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

func CreateSyncer(t *testing.T, blockchain blockchainShim, serverCfg *func(c *network.Config)) *Syncer {
	t.Helper()
	if serverCfg == nil {
		serverCfg = &defaultNetworkConfig
	}

	srv := network.CreateServer(t, *serverCfg)
	syncer := NewSyncer(hclog.NewNullLogger(), srv, blockchain)

	syncer.Start()
	t.Cleanup(func() {
		srv.Close()
	})

	return syncer
}

func WaitUntilPeerConnected(t *testing.T, syncer *Syncer, numPeer int, timeout time.Duration) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	t.Cleanup(func() {
		cancel()
	})

	_, err := tests.RetryUntilTimeout(ctx, func() (interface{}, bool) {
		num := len(syncer.peers)
		if num == numPeer {
			return nil, false
		}
		return nil, true
	})
	assert.NoError(t, err)
}

func NewRandomChain(t *testing.T, height int) blockchainShim {
	seed := rand.Intn(maxSeed)
	return blockchain.NewTestBlockchain(t, blockchain.NewTestHeaderChainWithSeed(nil, height, seed))
}

func NewRandomChainWithRandomHeight(t *testing.T) blockchainShim {
	return NewRandomChain(t, rand.Intn(maxHeight-1)+1)
}

func SetupSyncerNetwork(t *testing.T, chain blockchainShim, peerChains []blockchainShim) (syncer *Syncer, peerSyncers []*Syncer) {
	t.Helper()

	syncer = CreateSyncer(t, chain, nil)
	peerSyncers = make([]*Syncer, len(peerChains))
	for idx, peerChain := range peerChains {
		peerSyncers[idx] = CreateSyncer(t, peerChain, nil)
		network.MultiJoin(t, syncer.server, peerSyncers[idx].server)
	}
	WaitUntilPeerConnected(t, syncer, len(peerChains), 10*time.Second)
	return
}

func GenerateNewBlocks(t *testing.T, chain blockchainShim, num int) []*types.Block {
	t.Helper()

	currentHeight := chain.Header().Number
	oldHeaders := make([]*types.Header, currentHeight+1)
	for i := uint64(1); i <= currentHeight; i++ {
		var ok bool
		oldHeaders[i], ok = chain.GetHeaderByNumber(i)
		assert.Truef(t, ok, "chain should have header at %d, but empty", i)
	}
	headers := blockchain.NewTestHeaderFromChain(oldHeaders, num)
	return blockchain.HeadersToBlocks(headers[currentHeight+1:])
}

func TryPopBlock(t *testing.T, syncer *Syncer, peerID peer.ID, timeout time.Duration) (*types.Block, bool) {
	t.Helper()

	peer := syncer.peers[peerID]
	assert.NotNil(t, peer, "syncer doesn't have peer %s", peerID.String())

	blockCh := make(chan *types.Block, 1)
	go func() {
		blockCh <- peer.popBlock()
	}()

	select {
	case block := <-blockCh:
		return block, true
	case <-time.After(timeout):
		return nil, false
	}
}

func GetCurrentStatus(b blockchainShim) *Status {
	return &Status{
		Hash:       b.Header().Hash,
		Number:     b.Header().Number,
		Difficulty: b.CurrentTD(),
	}
}

func BlockToStatus(b *types.Block) *Status {
	return &Status{
		Hash:       b.Hash(),
		Number:     b.Number(),
		Difficulty: big.NewInt(0).SetUint64(b.Header.Difficulty),
	}
}
