package syncer

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/0xPolygon/polygon-edge/blockchain"
	"github.com/0xPolygon/polygon-edge/network"
	"github.com/0xPolygon/polygon-edge/network/event"
	"github.com/0xPolygon/polygon-edge/syncer/proto"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p-core/peer"
	"google.golang.org/protobuf/types/known/emptypb"
)

const (
	SyncPeerClientLoggerName = "sync-peer-client"
	statusTopicName          = "syncer/status/0.1"
	defaultTimeoutForStatus  = 10 * time.Second
)

type syncPeerClient struct {
	logger     hclog.Logger
	network    Network
	blockchain Blockchain

	subscription           blockchain.Subscription
	topic                  *network.Topic
	id                     string // node ID
	peerStatusUpdateCh     chan *NoForkPeer
	peerConnectionUpdateCh chan *event.PeerEvent
}

func NewSyncPeerClient(
	logger hclog.Logger,
	network Network,
	blockchain Blockchain,
) SyncPeerClient {
	return &syncPeerClient{
		logger:                 logger.Named(SyncPeerClientLoggerName),
		network:                network,
		blockchain:             blockchain,
		id:                     network.AddrInfo().ID.String(),
		peerStatusUpdateCh:     make(chan *NoForkPeer, 1),
		peerConnectionUpdateCh: make(chan *event.PeerEvent, 1),
	}
}

func (m *syncPeerClient) Start() error {
	go m.startNewBlockProcess()
	go m.startPeerEventProcess()

	if err := m.startGossip(); err != nil {
		return err
	}

	return nil
}

func (m *syncPeerClient) Close() {
	if m.subscription != nil {
		m.subscription.Close()

		m.subscription = nil
	}

	close(m.peerStatusUpdateCh)
	close(m.peerConnectionUpdateCh)
}

func (m *syncPeerClient) GetPeerStatus(peerID peer.ID) (*NoForkPeer, error) {
	clt, err := m.newSyncPeerClient(peerID)
	if err != nil {
		return nil, err
	}

	timeoutCtx, cancel := context.WithTimeout(context.Background(), defaultTimeoutForStatus)
	defer cancel()

	status, err := clt.GetStatus(timeoutCtx, &emptypb.Empty{})
	if err != nil {
		return nil, err
	}

	return &NoForkPeer{
		ID:       peerID,
		Number:   status.Number,
		Distance: m.network.GetPeerDistance(peerID),
	}, nil
}

// GetConnectedPeerStatuses returns the statuses of the all connecting nodes
func (m *syncPeerClient) GetConnectedPeerStatuses() []*NoForkPeer {
	var (
		ps            = m.network.Peers()
		syncPeers     = make([]*NoForkPeer, 0, len(ps))
		syncPeersLock sync.Mutex
		wg            sync.WaitGroup
	)

	for _, p := range ps {
		p := p

		wg.Add(1)

		go func() {
			defer wg.Done()

			peerID := p.Info.ID

			status, err := m.GetPeerStatus(peerID)
			if err != nil {
				m.logger.Warn("failed to get status from a peer, skip", "id", peerID, "err", err)
			}

			syncPeersLock.Lock()

			syncPeers = append(syncPeers, status)

			syncPeersLock.Unlock()
		}()
	}

	wg.Wait()

	return syncPeers
}

func (m *syncPeerClient) GetPeerStatusUpdateCh() <-chan *NoForkPeer {
	return m.peerStatusUpdateCh
}

func (m *syncPeerClient) GetPeerConnectionUpdateEventCh() <-chan *event.PeerEvent {
	return m.peerConnectionUpdateCh
}

func (m *syncPeerClient) startGossip() error {
	topic, err := m.network.NewTopic(statusTopicName, &proto.SyncPeerStatus{})
	if err != nil {
		return err
	}

	if err := topic.Subscribe(m.handleStatusUpdate); err != nil {
		return fmt.Errorf("unable to subscribe to gossip topic, %w", err)
	}

	m.topic = topic

	return nil
}

func (m *syncPeerClient) handleStatusUpdate(obj interface{}, from peer.ID) {
	status, ok := obj.(*proto.SyncPeerStatus)
	if !ok {
		m.logger.Error("failed to cast gossiped message to txn")

		return
	}

	if !m.network.IsConnected(from) {
		if m.id != from.String() {
			m.logger.Warn("received status from non-connected peer, ignore", "id", from)
		}

		return
	}

	m.peerStatusUpdateCh <- &NoForkPeer{
		ID:       from,
		Number:   status.Number,
		Distance: m.network.GetPeerDistance(from),
	}
}

func (m *syncPeerClient) startNewBlockProcess() {
	m.subscription = m.blockchain.SubscribeEvents()

	for event := range m.subscription.GetEventCh() {
		if event.Type == blockchain.EventHead && len(event.NewChain) > 0 {
			latest := m.blockchain.Header()
			if latest == nil {
				return
			}

			// Publish status
			if err := m.topic.Publish(&proto.SyncPeerStatus{
				Number: latest.Number,
			}); err != nil {
				m.logger.Warn("failed to publish status", "err", err)
			}
		}
	}
}

func (m *syncPeerClient) startPeerEventProcess() {
	peerEventCh, err := m.network.SubscribeCh()
	if err != nil {
		m.logger.Error("failed to subscribe", "err", err)

		return
	}

	for e := range peerEventCh {
		switch e.Type {
		case event.PeerConnected, event.PeerDisconnected:
			m.peerConnectionUpdateCh <- e
		}
	}
}

func (m *syncPeerClient) CloseStream(peerID peer.ID) error {
	return m.network.CloseProtocolStream(SyncerProto, peerID)
}

func (m *syncPeerClient) GetBlocks(
	ctx context.Context,
	peerID peer.ID,
	from uint64,
) (<-chan *types.Block, error) {
	clt, err := m.newSyncPeerClient(peerID)
	if err != nil {
		return nil, fmt.Errorf("failed to create sync peer client: %w", err)
	}

	stream, err := clt.GetBlocks(ctx, &proto.GetBlocksRequest{
		From: from,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open GetBlocks stream: %w", err)
	}

	blockCh := make(chan *types.Block, 1)

	go func() {
		defer close(blockCh)

		for {
			protoBlock, err := stream.Recv()
			if err != nil {
				break
			}

			block, err := fromProto(protoBlock)
			if err != nil {
				m.logger.Warn("failed to decode a block from peer", "peerID", peerID, "err", err)

				break
			}

			blockCh <- block
		}
	}()

	return blockCh, nil
}

func (m *syncPeerClient) newSyncPeerClient(peerID peer.ID) (proto.SyncPeerClient, error) {
	conn, err := m.network.NewProtoConnection(SyncerProto, peerID)
	if err != nil {
		return nil, fmt.Errorf("failed to open a stream, err %w", err)
	}

	m.network.SaveProtocolStream(SyncerProto, conn, peerID)

	return proto.NewSyncPeerClient(conn), nil
}

func fromProto(protoBlock *proto.Block) (*types.Block, error) {
	block := &types.Block{}
	if err := block.UnmarshalRLP(protoBlock.Block); err != nil {
		return nil, err
	}

	return block, nil
}
