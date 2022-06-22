package syncer

import (
	"context"
	"fmt"
	"math/big"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/blockchain"
	"github.com/0xPolygon/polygon-edge/helper/progress"
	"github.com/0xPolygon/polygon-edge/network/event"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/stretchr/testify/assert"
)

type mockProgression struct {
	startingBlock uint64
	highestBlock  uint64
}

func (m *mockProgression) StartProgression(startingBlock uint64, subscription blockchain.Subscription) {
	m.startingBlock = startingBlock
}

func (m *mockProgression) UpdateHighestProgression(highestBlock uint64) {
	m.highestBlock = highestBlock
}

func (m *mockProgression) GetProgression() *progress.Progression {
	// Syncer doesn't use this method. It just exports
	return nil
}

type mockSyncPeerService struct{}

func (m *mockSyncPeerService) Start() {}

func (m *mockProgression) StopProgression() {}

type mockSyncPeerClient struct {
	getPeerStatusHandler                  func(peer.ID) (*NoForkPeer, error)
	getConnectedPeerStatusesHandler       func() []*NoForkPeer
	getBlocksHandler                      func(context.Context, peer.ID, uint64) (<-chan *types.Block, error)
	getBlockHandler                       func(context.Context, peer.ID, uint64) (*types.Block, error)
	getPeerStatusUpdateChHandler          func() <-chan *NoForkPeer
	getPeerConnectionUpdateEventChHandler func() <-chan *event.PeerEvent
}

func (m *mockSyncPeerClient) Start() error {
	return nil
}

func (m *mockSyncPeerClient) Close() {}

func (m *mockSyncPeerClient) GetPeerStatus(id peer.ID) (*NoForkPeer, error) {
	return m.getPeerStatusHandler(id)
}

func (m *mockSyncPeerClient) GetConnectedPeerStatuses() []*NoForkPeer {
	return m.getConnectedPeerStatusesHandler()
}

func (m *mockSyncPeerClient) GetBlocks(ctx context.Context, id peer.ID, start uint64) (<-chan *types.Block, error) {
	return m.getBlocksHandler(ctx, id, start)
}

func (m *mockSyncPeerClient) GetBlock(ctx context.Context, id peer.ID, start uint64) (*types.Block, error) {
	return m.getBlockHandler(ctx, id, start)
}

func (m *mockSyncPeerClient) GetPeerStatusUpdateCh() <-chan *NoForkPeer {
	return m.getPeerStatusUpdateChHandler()
}

func (m *mockSyncPeerClient) GetPeerConnectionUpdateEventCh() <-chan *event.PeerEvent {
	return m.getPeerConnectionUpdateEventChHandler()
}

func GetAllElementsFromPeerMap(t *testing.T, p *PeerMap) []*NoForkPeer {
	t.Helper()

	peers := make([]*NoForkPeer, 0, 3)
	p.Range(func(key, value interface{}) bool {
		peer, ok := value.(*NoForkPeer)
		assert.True(t, ok)

		peers = append(peers, peer)

		return true
	})

	return peers
}

func SortPeerStatuses(peerStatuses []*NoForkPeer) []*NoForkPeer {
	sort.Slice(peerStatuses, func(p, q int) bool {
		return peerStatuses[p].Number < peerStatuses[q].Number
	})

	return peerStatuses
}

func NewTestSyncer(
	network Network,
	blockchain Blockchain,
	blockTimeout time.Duration,
	mockSyncPeerClient *mockSyncPeerClient,
) *syncer {
	return &syncer{
		logger:          hclog.NewNullLogger(),
		blockchain:      blockchain,
		syncProgression: &mockProgression{},
		syncPeerService: &mockSyncPeerService{},
		syncPeerClient:  mockSyncPeerClient,
		blockTimeout:    blockTimeout,
		newStatus:       make(chan struct{}),
		peerMap:         new(PeerMap),
	}
}

// Test whether Syncer calls GetConnectedPeerStatuses and initialize peerMap
func Test_initializePeerMap(t *testing.T) {
	peerStatuses := []*NoForkPeer{
		{
			ID:       peer.ID("A"),
			Number:   10,
			Distance: big.NewInt(10),
		},
		{
			ID:       peer.ID("B"),
			Number:   20,
			Distance: big.NewInt(20),
		},
		{
			ID:       peer.ID("C"),
			Number:   30,
			Distance: big.NewInt(30),
		},
	}

	runTest := func(t *testing.T, syncer *syncer) {
		t.Helper()

		syncer.initializePeerMap()

		peerMapStatuses := GetAllElementsFromPeerMap(t, syncer.peerMap)

		// no need to check order
		peerMapStatuses = SortPeerStatuses(peerMapStatuses)

		assert.Equal(t, peerStatuses, peerMapStatuses)
	}

	t.Run("initialize peerMap by GetConnectedPeerStatuses", func(t *testing.T) {
		runTest(t, NewTestSyncer(
			nil,
			nil,
			0,
			&mockSyncPeerClient{
				getConnectedPeerStatusesHandler: func() []*NoForkPeer {
					return peerStatuses
				},
				getPeerStatusUpdateChHandler: func() <-chan *NoForkPeer {
					ch := make(chan *NoForkPeer)
					// no update
					close(ch)

					return ch
				},
			},
		))
	})

	t.Run("update peerMap by GetPeerStatusUpdateCh", func(t *testing.T) {
		// number of signals to newStatus
		newStatusCount := 0

		syncer := NewTestSyncer(
			nil,
			nil,
			0,
			&mockSyncPeerClient{
				getConnectedPeerStatusesHandler: func() []*NoForkPeer {
					return []*NoForkPeer{}
				},
				getPeerStatusUpdateChHandler: func() <-chan *NoForkPeer {
					ch := make(chan *NoForkPeer, len(peerStatuses))

					for _, s := range peerStatuses {
						ch <- s
					}

					close(ch)

					return ch
				},
			},
		)

		var wg sync.WaitGroup
		wg.Add(1)

		go func() {
			defer wg.Done()

			for range syncer.newStatus {
				newStatusCount++
			}
		}()

		runTest(t, syncer)

		close(syncer.newStatus)

		wg.Wait()

		assert.Equal(t, len(peerStatuses), newStatusCount)
	})
}

func Test_startPeerDisconnectEventProcess(t *testing.T) {
	tests := []struct {
		name            string
		events          []*event.PeerEvent
		statuses        map[peer.ID]*NoForkPeer
		expectedPeerMap []*NoForkPeer
	}{
		{
			name: "should add peer to PeerMap after PeerConnected",
			events: []*event.PeerEvent{
				{
					PeerID: peer.ID("A"),
					Type:   event.PeerConnected,
				},
				{
					PeerID: peer.ID("B"),
					Type:   event.PeerConnected,
				},
			},
			statuses: map[peer.ID]*NoForkPeer{
				peer.ID("A"): {
					ID:       peer.ID("A"),
					Number:   10,
					Distance: big.NewInt(10),
				},
				peer.ID("B"): {
					ID:       peer.ID("B"),
					Number:   20,
					Distance: big.NewInt(20),
				},
			},
			expectedPeerMap: []*NoForkPeer{
				{
					ID:       peer.ID("A"),
					Number:   10,
					Distance: big.NewInt(10),
				},
				{
					ID:       peer.ID("B"),
					Number:   20,
					Distance: big.NewInt(20),
				},
			},
		},
		{
			name: "should remove peer to PeerMap after PeerDisconnected",
			events: []*event.PeerEvent{
				{
					PeerID: peer.ID("A"),
					Type:   event.PeerConnected,
				},
				{
					PeerID: peer.ID("A"),
					Type:   event.PeerDisconnected,
				},
			},
			statuses: map[peer.ID]*NoForkPeer{
				peer.ID("A"): {
					ID:       peer.ID("A"),
					Number:   10,
					Distance: big.NewInt(10),
				},
			},
			expectedPeerMap: []*NoForkPeer{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			syncer := NewTestSyncer(
				nil,
				nil,
				0,
				&mockSyncPeerClient{
					getPeerConnectionUpdateEventChHandler: func() <-chan *event.PeerEvent {
						ch := make(chan *event.PeerEvent, len(test.events))

						for _, e := range test.events {
							ch <- e

							time.Sleep(time.Millisecond * 500)
						}

						close(ch)

						return ch
					},
					getPeerStatusHandler: func(i peer.ID) (*NoForkPeer, error) {
						status, ok := test.statuses[i]
						if !ok {
							return nil, fmt.Errorf("peer %s didn't return status", i)
						}

						return status, nil
					},
				},
			)

			syncer.startPeerDisconnectEventProcess()

			peerMapStatuses := GetAllElementsFromPeerMap(t, syncer.peerMap)

			// no need to check order
			peerMapStatuses = SortPeerStatuses(peerMapStatuses)

			assert.Equal(t, test.expectedPeerMap, peerMapStatuses)
		})
	}
}
