package syncer

import (
	"sync"
	"testing"

	"github.com/0xPolygon/polygon-edge/network"
	"github.com/0xPolygon/polygon-edge/network/event"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
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

func newTestSyncPeerClient(t *testing.T, network Network, blockchain Blockchain) *syncPeerClient {
	t.Helper()

	return &syncPeerClient{
		logger:                 hclog.NewNullLogger(),
		network:                network,
		blockchain:             blockchain,
		id:                     string(network.AddrInfo().ID),
		peerStatusUpdateCh:     make(chan *NoForkPeer, 1),
		peerConnectionUpdateCh: make(chan *event.PeerEvent, 1),
	}
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
	clientSrv := newTestNetwork(t)
	client := newTestSyncPeerClient(t, clientSrv, nil)

	peerLatest := uint64(10)
	_, peerSrv := createTestSyncerService(t, &mockBlockchain{
		headerHandler: func() *types.Header {
			return &types.Header{
				Number: peerLatest,
			}
		},
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
	clientSrv := newTestNetwork(t)
	client := newTestSyncPeerClient(t, clientSrv, nil)

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
			headerHandler: func() *types.Header {
				return &types.Header{
					Number: latest,
				}
			},
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

func TestPeerConnectionUpdateEventCh(t *testing.T) {
	clientSrv := newTestNetwork(t)
	client := newTestSyncPeerClient(t, clientSrv, nil)

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

// Gossip

// GetBlocks
