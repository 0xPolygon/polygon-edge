package nofork

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/0xPolygon/polygon-edge/blockchain"
	"github.com/0xPolygon/polygon-edge/network/grpc"
	"github.com/0xPolygon/polygon-edge/syncer/peers"
	"github.com/0xPolygon/polygon-edge/syncer/peers/nofork/proto"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p-core/peer"
	"google.golang.org/protobuf/types/known/emptypb"
)

const (
	SyncPeersManagerLoggerName = "sync-peers-manager"
)

type SyncPeersManager struct {
	logger     hclog.Logger
	network    Network
	blockchain Blockchain
	server     *SyncPeerService

	peerHeap     *peers.PeerHeap
	subscription blockchain.Subscription
	id           string // node ID
}

func NewSyncPeersManager(
	logger hclog.Logger,
	network Network,
	blockchain Blockchain,
) peers.SyncPeers {
	return &SyncPeersManager{
		logger:     logger.Named(SyncPeersManagerLoggerName),
		network:    network,
		blockchain: blockchain,
		server: NewSyncPeerService(
			logger,
			blockchain,
			network,
		),
		id: network.AddrInfo().ID.String(),
	}
}

func (m *SyncPeersManager) Start() {
	m.server.Start()

	m.initializePeerHeap()
	go m.startNewBlockProcess()
	go m.startNewPeerStatusProcess()
}

func (m *SyncPeersManager) Close() {
	if m.subscription != nil {
		m.subscription.Close()

		m.subscription = nil
	}
}

func (m *SyncPeersManager) initializePeerHeap() {
	ps := m.network.Peers()
	syncPeers := make([]peers.Peer, 0, len(ps))

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
			id:       status.Id,
			Number:   status.Number,
			Distance: m.network.GetPeerDistance(peer.ID(status.Id)),
		})
	}

	m.peerHeap = peers.NewPeerHeap(syncPeers)
}

func (m *SyncPeersManager) startNewBlockProcess() {
	m.subscription = m.blockchain.SubscribeEvents()

	for event := range m.subscription.GetEventCh() {
		if event.Type == blockchain.EventHead && len(event.NewChain) > 0 {
			// Latest Block Number is updated
			m.broadcastStatus()
		}
	}
}

func (m *SyncPeersManager) startNewPeerStatusProcess() {
	for peerStatus := range m.server.PeerStatusCh() {
		m.peerHeap.Put(peerStatus)
	}
}

func (m *SyncPeersManager) broadcastStatus() {
	latest := m.blockchain.Header()
	if latest == nil {
		return
	}

	for _, p := range m.network.Peers() {
		peerID := p.Info.ID

		clt, err := m.newSyncPeerClient(peerID)
		if err != nil {
			m.logger.Warn("failed to create sync peer client, skip", "id", peerID, "err", err)

			continue
		}

		clt.NotifyStatus(context.Background(), &proto.Status{
			Id:     m.id,
			Number: latest.Number,
		})
	}
}

func (m *SyncPeersManager) GetBlocks(
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

func (m *SyncPeersManager) GetBlock(
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

func (m *SyncPeersManager) BestPeer() *peers.BestPeer {
	p := m.peerHeap.BestPeer()
	if p == nil {
		return nil
	}

	np, ok := p.(*NoForkPeer)
	if !ok {
		return nil
	}

	return &peers.BestPeer{
		ID:     np.ID(),
		Number: np.Number,
	}
}

func (m *SyncPeersManager) newSyncPeerClient(peerID peer.ID) (proto.SyncPeerClient, error) {
	stream, err := m.network.NewStream(peers.SyncerProto, peerID)
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
