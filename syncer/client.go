package syncer

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/0xPolygon/polygon-edge/blockchain"
	"github.com/0xPolygon/polygon-edge/network"
	"github.com/0xPolygon/polygon-edge/network/event"
	"github.com/0xPolygon/polygon-edge/syncer/proto"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p-core/peer"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	// XXX: Experiment Code Begins
	"google.golang.org/grpc/encoding/gzip"
	// XXX: Experiment Code Ends
)

const (
	SyncPeersManagerLoggerName = "sync-peers-manager"
	statusTopicName            = "syncer/status/0.1"
	defaultTimeoutForStatus    = 10 * time.Second
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

	// XXX: Experiment Code Begins
	chunkSize  uint64
	enableGZip bool
	// XXX: Experiment Code Ends
}

func NewSyncPeerClient(
	logger hclog.Logger,
	network Network,
	blockchain Blockchain,
) SyncPeerClient {
	// XXX: Experiment Code Begins
	var (
		chunkSize  = uint64(1)
		enableGZip = false
	)

	if e := os.Getenv("CHUNK_SIZE"); e != "" {
		n, err := strconv.ParseInt(e, 10, 64)
		if err != nil {
			logger.Error("failed to parse uint64 string", "CHUNK_SIZE", e)

			return nil
		}

		if n < 0 {
			logger.Error("CHUNK_SIZE must be positive", "n", n)

			return nil
		}

		chunkSize = uint64(n)
	}

	if e := os.Getenv("ENABLE_GZIP"); e != "" {
		enableGZip = strings.ToLower(e) == "true"
	}

	fmt.Printf("Chunk Size => %d\n", chunkSize)
	fmt.Printf("Enable GZip => %t\n", enableGZip)
	// XXX: Experiment Code Ends

	return &syncPeerClient{
		logger:                 logger.Named(SyncPeersManagerLoggerName),
		network:                network,
		blockchain:             blockchain,
		id:                     network.AddrInfo().ID.String(),
		peerStatusUpdateCh:     make(chan *NoForkPeer, 1),
		peerConnectionUpdateCh: make(chan *event.PeerEvent, 1),

		// XXX: Experiment Code Begins
		chunkSize:  chunkSize,
		enableGZip: enableGZip,
		// XXX: Experiment Code Ends
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

func (m *syncPeerClient) GetConnectedPeerStatuses() []*NoForkPeer {
	var (
		ps            = m.network.Peers()
		syncPeers     = make([]*NoForkPeer, 0, len(ps))
		syncPeersLock = sync.Mutex{}
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

	opts := []grpc.CallOption{}
	if m.enableGZip {
		opts = append(opts, grpc.UseCompressor(gzip.Name))
	}

	stream, err := clt.GetBlocks(ctx, &proto.GetBlocksRequest{
		From: from,
		// XXX: Experiment Code Begins
		Chunk: m.chunkSize,
		// XXX: Experiment Code Ends
	}, opts...)

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

			// block, err := fromProto(protoBlock)
			// if err != nil {
			// 	m.logger.Warn("failed to decode a block from peer", "peerID", peerID, "err", err)

			// 	break
			// }

			// blockCh <- block

			// Experiment Code Begins
			blocks := &types.Blocks{}
			if err := blocks.UnmarshalRLP(protoBlock.Block); err != nil {
				m.logger.Error("failed to unmarshal block", "peerID", peerID, "err", err)

				break
			}

			for _, b := range *blocks {
				blockCh <- b
			}
			// Experiment Code Ends
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
