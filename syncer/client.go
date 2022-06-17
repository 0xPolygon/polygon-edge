package syncer

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/0xPolygon/polygon-edge/blockchain"
	"github.com/0xPolygon/polygon-edge/network"
	"github.com/0xPolygon/polygon-edge/network/grpc"
	"github.com/0xPolygon/polygon-edge/syncer/proto"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p-core/peer"
	"google.golang.org/protobuf/types/known/emptypb"
)

const (
	SyncPeersManagerLoggerName = "sync-peers-manager"
	statusTopicName            = "syncer/status/0.1"
)

type syncPeerClient struct {
	logger     hclog.Logger
	network    Network
	blockchain Blockchain

	subscription       blockchain.Subscription
	topic              *network.Topic
	id                 string // node ID
	updatePeerStatusCh chan *NoForkPeer
}

func NewSyncPeerClient(
	logger hclog.Logger,
	network Network,
	blockchain Blockchain,
) SyncPeerClient {
	return &syncPeerClient{
		logger:             logger.Named(SyncPeersManagerLoggerName),
		network:            network,
		blockchain:         blockchain,
		id:                 network.AddrInfo().ID.String(),
		updatePeerStatusCh: make(chan *NoForkPeer, 1),
	}
}

func (m *syncPeerClient) Start() {
	go m.startNewBlockProcess()

	// TODO: return error
	_ = m.startGossip()
}

func (m *syncPeerClient) Close() {
	if m.subscription != nil {
		m.subscription.Close()

		m.subscription = nil
	}

	close(m.updatePeerStatusCh)
}

func (m *syncPeerClient) GetConnectedPeerStatuses() []*NoForkPeer {
	ps := m.network.Peers()
	syncPeers := make([]*NoForkPeer, 0, len(ps))

	for _, p := range ps {
		peerID := p.Info.ID

		clt, err := m.newSyncPeerClient(peerID)
		if err != nil {
			m.logger.Warn("failed to create sync peer client, skip", "id", peerID, "err", err)

			continue
		}

		status, err := clt.GetStatus(context.Background(), &emptypb.Empty{})
		if err != nil {
			m.logger.Warn("failed to get sync status from peer, skip", "id", peerID, "err", err)

			continue
		}

		syncPeers = append(syncPeers, &NoForkPeer{
			ID:       status.Id,
			Number:   status.Number,
			Distance: m.network.GetPeerDistance(peer.ID(status.Id)),
		})
	}

	return syncPeers
}

func (m *syncPeerClient) GetPeerStatusChangeCh() <-chan *NoForkPeer {
	return m.updatePeerStatusCh
}

func (m *syncPeerClient) startGossip() error {
	topic, err := m.network.NewTopic(statusTopicName, &proto.Status{})
	if err != nil {
		return err
	}

	if err := topic.Subscribe(m.handleStatusUpdate); err != nil {
		return fmt.Errorf("unable to subscribe to gossip topic, %w", err)
	}

	m.topic = topic

	return nil
}

func (m *syncPeerClient) handleStatusUpdate(obj interface{}) {
	status, ok := obj.(*proto.Status)
	if !ok {
		m.logger.Error("failed to cast gossiped message to txn")

		return
	}

	if !m.network.IsConnected(peer.ID(status.Id)) {
		m.logger.Warn("received status from non-connected peer, ignore", "id", status.Id)

		return
	}

	m.updatePeerStatusCh <- &NoForkPeer{
		ID:     status.Id,
		Number: status.Number,
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
			m.topic.Publish(&proto.Status{
				Id:     string(m.network.AddrInfo().ID),
				Number: latest.Number,
			})
		}
	}
}

func (m *syncPeerClient) GetBlocks(
	ctx context.Context,
	peerID string,
	from uint64,
) (<-chan *types.Block, error) {
	clt, err := m.newSyncPeerClient(peer.ID(peerID))
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
			if errors.Is(err, io.EOF) {
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

func (m *syncPeerClient) GetBlock(
	ctx context.Context,
	peerID string,
	number uint64,
) (*types.Block, error) {
	clt, err := m.newSyncPeerClient(peer.ID(peerID))
	if err != nil {
		return nil, fmt.Errorf("failed to create sync peer client: %w", err)
	}

	protoBlock, err := clt.GetBlock(ctx, &proto.GetBlockRequest{
		Number: number,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch block from peer: %w", err)
	}

	block, err := fromProto(protoBlock)
	if err != nil {
		return nil, fmt.Errorf("failed to decode a block from peer: %w", err)
	}

	return block, nil
}

func (m *syncPeerClient) newSyncPeerClient(peerID peer.ID) (proto.SyncPeerClient, error) {
	stream, err := m.network.NewStream(SyncerProto, peerID)
	if err != nil {
		return nil, fmt.Errorf("failed to open a stream, err %w", err)
	}

	conn := grpc.WrapClient(stream)

	return proto.NewSyncPeerClient(conn), nil
}

func fromProto(protoBlock *proto.Block) (*types.Block, error) {
	block := &types.Block{}
	if err := block.UnmarshalRLP(protoBlock.Block); err != nil {
		return nil, err
	}

	return block, nil
}
