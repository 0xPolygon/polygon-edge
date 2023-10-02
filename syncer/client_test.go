package syncer

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/0xPolygon/polygon-edge/blockchain"
	"github.com/0xPolygon/polygon-edge/network"
	"github.com/0xPolygon/polygon-edge/network/event"
	"github.com/0xPolygon/polygon-edge/network/grpc"
	"github.com/0xPolygon/polygon-edge/syncer/proto"
	"github.com/0xPolygon/polygon-edge/types"
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
	client := &syncPeerClient{
		logger:                 hclog.NewNullLogger(),
		network:                network,
		blockchain:             blockchain,
		id:                     network.AddrInfo().ID.String(),
		peerStatusUpdateCh:     make(chan *NoForkPeer, 1),
		peerConnectionUpdateCh: make(chan *event.PeerEvent, 1),
		closeCh:                make(chan struct{}),
	}

	// need to register protocol
	network.RegisterProtocol(syncerProto, grpc.NewGrpcStream())

	return client
}

func createTestSyncerService(t *testing.T, chain Blockchain) (*syncPeerService, *network.Server) {
	t.Helper()

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
	close(client.closeCh)

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
	t.Skip()
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

	// enable peers to send own status via gossip
	peerClient1.EnablePublishingPeerStatus()
	peerClient2.EnablePublishingPeerStatus()

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

	// push latest block number to blockchain subscription
	pushSubscription := func(sub *blockchain.MockSubscription, latest uint64) {
		sub.Push(&blockchain.Event{
			NewChain: []*types.Header{
				{
					Number: latest,
				},
			},
		})
	}

	// peer1 and peer2 emit Blockchain event
	// they should publish their status via gossip
	pushSubscription(subscription1, peerLatest1)
	pushSubscription(subscription2, peerLatest2)

	// wait until 2 messages are propagated
	wgForGossip.Wait()

	// close to terminate goroutine
	client.Close()

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

// Make sure the peer shouldn't emit status if the shouldEmitBlocks flag is set.
// The subtests cannot contain t.Parallel() due to how
// the test is organized
//
//nolint:tparallel
func Test_shouldEmitBlocks(t *testing.T) {
	t.Parallel()

	var (
		// network layer
		clientSrv = newTestNetwork(t)
		peerSrv   = newTestNetwork(t)

		clientLatest = uint64(10)

		subscription = blockchain.NewMockSubscription()

		client = newTestSyncPeerClient(clientSrv, &mockBlockchain{
			subscription:  subscription,
			headerHandler: newSimpleHeaderHandler(clientLatest),
		})
	)

	t.Cleanup(func() {
		clientSrv.Close()
		peerSrv.Close()
		client.Close()
	})

	err := network.JoinAndWaitMultiple(
		network.DefaultJoinTimeout,
		clientSrv,
		peerSrv,
	)

	assert.NoError(t, err)

	// start gossip
	assert.NoError(t, client.startGossip())

	// start to subscribe blockchain events
	go client.startNewBlockProcess()

	// push latest block number to blockchain subscription
	pushSubscription := func(sub *blockchain.MockSubscription, latest uint64) {
		sub.Push(&blockchain.Event{
			NewChain: []*types.Header{
				{
					Number: latest,
				},
			},
		})
	}

	waitForContext := func(ctx context.Context) bool {
		select {
		case <-ctx.Done():
			return true
		case <-time.After(5 * time.Second):
			return false
		}
	}

	// create topic & subscribe in peer
	topic, err := peerSrv.NewTopic(statusTopicName, &proto.SyncPeerStatus{})
	assert.NoError(t, err)

	testGossip := func(t *testing.T, shouldEmit bool) {
		t.Helper()

		// context to be canceled when receiving status
		receiveContext, cancelContext := context.WithCancel(context.Background())
		defer cancelContext()

		assert.NoError(t, topic.Subscribe(func(_ interface{}, id peer.ID) {
			cancelContext()
		}))

		// need to wait for a few seconds to propagate subscribing
		time.Sleep(2 * time.Second)

		if shouldEmit {
			client.EnablePublishingPeerStatus()
		} else {
			client.DisablePublishingPeerStatus()
		}

		pushSubscription(subscription, clientLatest)

		canceled := waitForContext(receiveContext)

		assert.Equal(t, shouldEmit, canceled)
	}

	t.Run("should send own status via gossip if shouldEmitBlocks is set", func(t *testing.T) {
		testGossip(t, true)
	})

	t.Run("shouldn't send own status via gossip if shouldEmitBlocks is reset", func(t *testing.T) {
		testGossip(t, false)
	})
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

	blockStream, err := client.GetBlocks(peerSrv.AddrInfo().ID, syncFrom, 5*time.Second)
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

func Test_EmitMultipleBlocks(t *testing.T) {
	t.Parallel()

	var (
		// network layer
		clientSrv = newTestNetwork(t)
		peerSrv   = newTestNetwork(t)

		clientLatest = uint64(10)

		subscription = blockchain.NewMockSubscription()

		client = newTestSyncPeerClient(clientSrv, &mockBlockchain{
			subscription:  subscription,
			headerHandler: newSimpleHeaderHandler(clientLatest),
		})
	)

	t.Cleanup(func() {
		clientSrv.Close()
		peerSrv.Close()
		client.Close()
	})

	err := network.JoinAndWaitMultiple(
		network.DefaultJoinTimeout,
		clientSrv,
		peerSrv,
	)

	require.NoError(t, err)

	// start gossip
	require.NoError(t, client.startGossip())

	// start to subscribe blockchain events
	go client.startNewBlockProcess()

	// push latest block number to blockchain subscription
	pushSubscription := func(sub *blockchain.MockSubscription, latest uint64) {
		sub.Push(&blockchain.Event{
			NewChain: []*types.Header{
				{
					Number: latest,
				},
			},
		})
	}

	waitForGossip := func(wg *sync.WaitGroup) bool {
		c := make(chan struct{})

		go func() {
			defer close(c)
			wg.Wait()
		}()

		select {
		case <-c:
			return true
		case <-time.After(5 * time.Second):
			return false
		}
	}

	// create topic & subscribe in peer
	topic, err := peerSrv.NewTopic(statusTopicName, &proto.SyncPeerStatus{})
	assert.NoError(t, err)

	testGossip := func(t *testing.T, blocksNum int) {
		t.Helper()

		var wgForGossip sync.WaitGroup

		wgForGossip.Add(blocksNum)

		require.NoError(t, topic.Subscribe(func(_ interface{}, _ peer.ID) {
			wgForGossip.Done()
		}))

		// need to wait for a few seconds to propagate subscribing
		time.Sleep(2 * time.Second)
		client.EnablePublishingPeerStatus()

		go func() {
			for i := 0; i < blocksNum; i++ {
				pushSubscription(subscription, clientLatest+uint64(i))
			}
		}()

		gossiped := waitForGossip(&wgForGossip)

		require.Equal(t, true, gossiped)
	}

	t.Run("should receive all blocks", func(t *testing.T) {
		t.Parallel()
		testGossip(t, 4)
	})
}
