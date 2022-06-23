package syncer

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/blockchain"
	"github.com/0xPolygon/polygon-edge/network"
	"github.com/0xPolygon/polygon-edge/network/event"
	"github.com/0xPolygon/polygon-edge/syncer/proto"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/stretchr/testify/assert"
)

var (
	networkConfig = func(c *network.Config) {
		c.NoDiscover = true
	}
)

func newTestNetwork(t *testing.T) *network.Server {
	t.Helper()

	srv, err := network.CreateServer(&network.CreateServerParams{
		ConfigCallback: networkConfig,
	})

	assert.NoError(t, err)

	return srv
}

func newTestSyncPeerClient(network Network, blockchain Blockchain) *syncPeerClient {
	return &syncPeerClient{
		logger:                 hclog.NewNullLogger(),
		network:                network,
		blockchain:             blockchain,
		id:                     network.AddrInfo().ID.String(),
		peerStatusUpdateCh:     make(chan *NoForkPeer, 1),
		peerConnectionUpdateCh: make(chan *event.PeerEvent, 1),
	}
}

func createTestSyncerService(t *testing.T, chain Blockchain) (*syncPeerService, *network.Server) {
	srv := newTestNetwork(t)

	service := &syncPeerService{
		blockchain: chain,
		network:    srv,
	}

	service.Start()

	return service, srv
}

func TestGetPeerStatus(t *testing.T) {
	t.Parallel()

	clientSrv := newTestNetwork(t)
	client := newTestSyncPeerClient(clientSrv, nil)

	peerLatest := uint64(10)
	_, peerSrv := createTestSyncerService(t, &mockBlockchain{
		headerHandler: newSimpleHeaderHandler(peerLatest),
	})

	err := network.JoinAndWait(
		clientSrv,
		peerSrv,
		network.DefaultBufferTimeout,
		network.DefaultJoinTimeout,
	)

	assert.NoError(t, err)

	status, err := client.GetPeerStatus(peerSrv.AddrInfo().ID)
	assert.NoError(t, err)

	expected := &NoForkPeer{
		ID:       peerSrv.AddrInfo().ID,
		Number:   peerLatest,
		Distance: clientSrv.GetPeerDistance(peerSrv.AddrInfo().ID),
	}

	assert.Equal(t, expected, status)
}

func TestGetConnectedPeerStatuses(t *testing.T) {
	t.Parallel()

	clientSrv := newTestNetwork(t)
	client := newTestSyncPeerClient(clientSrv, nil)

	var (
		peerLatests = []uint64{
			30,
			20,
			10,
		}

		mutex        = sync.Mutex{}
		peerJoinErrs = make([]error, len(peerLatests))
		expected     = make([]*NoForkPeer, len(peerLatests))

		wg sync.WaitGroup
	)

	for idx, latest := range peerLatests {
		idx, latest := idx, latest

		_, peerSrv := createTestSyncerService(t, &mockBlockchain{
			headerHandler: newSimpleHeaderHandler(latest),
		})

		peerID := peerSrv.AddrInfo().ID

		wg.Add(1)
		go func() {
			defer wg.Done()

			mutex.Lock()
			defer mutex.Unlock()

			peerJoinErrs[idx] = network.JoinAndWait(
				clientSrv,
				peerSrv,
				network.DefaultBufferTimeout,
				network.DefaultJoinTimeout,
			)

			expected[idx] = &NoForkPeer{
				ID:       peerID,
				Number:   latest,
				Distance: clientSrv.GetPeerDistance(peerID),
			}
		}()
	}

	wg.Wait()

	for _, err := range peerJoinErrs {
		assert.NoError(t, err)
	}

	statuses := client.GetConnectedPeerStatuses()

	// no need to check order
	assert.Equal(t, expected, sortNoForkPeers(statuses))
}

func TestStatusPubSub(t *testing.T) {
	t.Parallel()

	clientSrv := newTestNetwork(t)
	client := newTestSyncPeerClient(clientSrv, nil)

	_, peerSrv := createTestSyncerService(t, &mockBlockchain{})
	peerID := peerSrv.AddrInfo().ID

	go client.startPeerEventProcess()

	// run goroutine to collect events
	var (
		events = []*event.PeerEvent{}
		wg     sync.WaitGroup
	)

	wg.Add(1)
	go func() {
		defer wg.Done()

		for event := range client.GetPeerConnectionUpdateEventCh() {
			events = append(events, event)
		}
	}()

	// connect
	err := network.JoinAndWait(
		clientSrv,
		peerSrv,
		network.DefaultBufferTimeout,
		network.DefaultJoinTimeout,
	)

	assert.NoError(t, err)

	// disconnect
	err = network.DisconnectAndWait(
		clientSrv,
		peerID,
		network.DefaultLeaveTimeout,
	)

	assert.NoError(t, err)

	// close channel and wait for events
	close(client.peerConnectionUpdateCh)

	wg.Wait()

	expected := []*event.PeerEvent{
		{
			PeerID: peerID,
			Type:   event.PeerConnected,
		},
		{
			PeerID: peerID,
			Type:   event.PeerDisconnected,
		},
	}

	assert.Equal(t, expected, events)
}

func TestPeerConnectionUpdateEventCh(t *testing.T) {
	t.Parallel()

	var (
		// network layer
		clientSrv = newTestNetwork(t)
		peerSrv1  = newTestNetwork(t)
		peerSrv2  = newTestNetwork(t)
		peerSrv3  = newTestNetwork(t) // to wait for gossipped message

		// latest block height
		peerLatest1 = uint64(10)
		peerLatest2 = uint64(20)

		// blockchain subscription
		subscription1 = blockchain.NewMockSubscription()
		subscription2 = blockchain.NewMockSubscription()

		// syncer client
		client = newTestSyncPeerClient(clientSrv, &mockBlockchain{
			subscription: &blockchain.MockSubscription{},
		})
		peerClient1 = newTestSyncPeerClient(peerSrv1, &mockBlockchain{
			subscription:  subscription1,
			headerHandler: newSimpleHeaderHandler(peerLatest1),
		})
		peerClient2 = newTestSyncPeerClient(peerSrv2, &mockBlockchain{
			subscription:  subscription2,
			headerHandler: newSimpleHeaderHandler(peerLatest2),
		})
	)

	t.Cleanup(func() {
		clientSrv.Close()
		peerSrv1.Close()
		peerSrv2.Close()
		peerSrv3.Close()

		// no need to call Close of Client because test closes it manually
		peerClient1.Close()
		peerClient2.Close()
	})

	// client <-> peer1
	// peer1  <-> peer2
	err := network.JoinAndWaitMultiple(
		network.DefaultJoinTimeout,
		clientSrv,
		peerSrv1,
		peerSrv1,
		peerSrv2,
		peerSrv2,
		peerSrv3,
	)

	assert.NoError(t, err)

	// start gossip
	assert.NoError(t, client.startGossip())
	assert.NoError(t, peerClient1.startGossip())
	assert.NoError(t, peerClient2.startGossip())

	// create topic
	topic, err := peerSrv3.NewTopic(statusTopicName, &proto.SyncPeerStatus{})
	assert.NoError(t, err)

	var wgForGossip sync.WaitGroup

	// 2 messages should be gossipped
	wgForGossip.Add(2)
	handler := func(_ interface{}, _ peer.ID) {
		wgForGossip.Done()
	}

	assert.NoError(t, topic.Subscribe(handler))

	// need to wait for a few seconds to propagate subscribing
	time.Sleep(2 * time.Second)

	// start to subscribe blockchain events
	go peerClient1.startNewBlockProcess()
	go peerClient2.startNewBlockProcess()

	// collect peer status changes
	var (
		wgForConnectingStatus sync.WaitGroup
		newStatuses           []*NoForkPeer
	)

	wgForConnectingStatus.Add(1)
	go func() {
		defer wgForConnectingStatus.Done()

		for status := range client.GetPeerStatusUpdateCh() {
			newStatuses = append(newStatuses, status)
		}
	}()

	// peer1 and peer2 emit Blockchain event
	// they should publish their status via gossip
	blockchainEvent := &blockchain.Event{
		NewChain: []*types.Header{
			{},
		},
	}

	subscription1.Push(blockchainEvent)
	subscription2.Push(blockchainEvent)

	// wait until 2 messages are propagated
	wgForGossip.Wait()

	// close to terminate goroutine
	close(client.peerStatusUpdateCh)

	// wait until collecting routine is done
	wgForConnectingStatus.Wait()

	// client connects to only peer1, then expects to have a status from peer1
	expected := []*NoForkPeer{
		{
			ID:       peerSrv1.AddrInfo().ID,
			Number:   peerLatest1,
			Distance: clientSrv.GetPeerDistance(peerSrv1.AddrInfo().ID),
		},
	}

	assert.Equal(t, expected, newStatuses)
}

func Test_syncPeerClient_GetBlocks(t *testing.T) {
	t.Parallel()

	clientSrv := newTestNetwork(t)
	client := newTestSyncPeerClient(clientSrv, nil)

	var (
		peerLatest = uint64(10)
		syncFrom   = uint64(1)
	)

	_, peerSrv := createTestSyncerService(t, &mockBlockchain{
		headerHandler: newSimpleHeaderHandler(peerLatest),
		getBlockByNumberHandler: func(u uint64, b bool) (*types.Block, bool) {
			if u <= 10 {
				return &types.Block{
					Header: &types.Header{
						Number: u,
					},
				}, true
			}

			return nil, false
		},
	})

	err := network.JoinAndWait(
		clientSrv,
		peerSrv,
		network.DefaultBufferTimeout,
		network.DefaultJoinTimeout,
	)

	assert.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	blockStream, err := client.GetBlocks(ctx, peerSrv.AddrInfo().ID, syncFrom)
	assert.NoError(t, err)

	blocks := make([]*types.Block, 0, peerLatest)
	for block := range blockStream {
		blocks = append(blocks, block)
	}

	// hash is calculated on unmarshaling
	expected := createMockBlocks(10)
	for _, b := range expected {
		b.Header.ComputeHash()
	}

	assert.Equal(t, expected, blocks)
}
