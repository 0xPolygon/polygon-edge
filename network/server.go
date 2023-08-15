package network

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/0xPolygon/polygon-edge/network/common"
	"github.com/0xPolygon/polygon-edge/network/dial"
	"github.com/0xPolygon/polygon-edge/network/discovery"
	"github.com/armon/go-metrics"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/p2p/security/noise"
	rawGrpc "google.golang.org/grpc"

	peerEvent "github.com/0xPolygon/polygon-edge/network/event"
	"github.com/0xPolygon/polygon-edge/secrets"
	"github.com/hashicorp/go-hclog"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/event"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/multiformats/go-multiaddr"
)

const (
	// peerOutboundBufferSize is the size of outbound messages to a peer buffers in go-libp2p-pubsub
	// we should have enough capacity of the queue
	// because we start dropping messages to a peer if the outbound queue is full
	peerOutboundBufferSize = 1024

	// validateBufferSize is the size of validate buffers in go-libp2p-pubsub
	// we should have enough capacity of the queue
	// because when queue is full, validation is throttled and new messages are dropped.
	validateBufferSize = 1024

	// networkMetrics is a prefix used for network-related metrics
	networkMetrics = "network"
)

const (
	defaultBucketSize = 20
	DefaultDialRatio  = 0.2

	DefaultLibp2pPort int = 1478

	MinimumBootNodes       int   = 1
	MinimumPeerConnections int64 = 1
)

var (
	ErrNoBootnodes  = errors.New("no bootnodes specified")
	ErrMinBootnodes = errors.New("minimum 1 bootnode is required")
)

type Server struct {
	logger hclog.Logger // the logger
	config *Config      // the base networking server configuration

	closeCh chan struct{} // the channel used for closing the networking server

	host  host.Host             // the libp2p host reference
	addrs []multiaddr.Multiaddr // the list of supported (bound) addresses

	peers     map[peer.ID]*PeerConnInfo // map of all peer connections
	peersLock sync.Mutex                // lock for the peer map

	dialQueue *dial.DialQueue // queue used to asynchronously connect to peers

	discovery *discovery.DiscoveryService // service used for discovering other peers

	protocols     map[string]Protocol // supported protocols
	protocolsLock sync.Mutex          // lock for the supported protocols map

	secretsManager secrets.SecretsManager // secrets manager for networking keys

	ps *pubsub.PubSub // reference to the networking PubSub service

	emitterPeerEvent event.Emitter // event emitter for listeners

	connectionCounts *ConnectionInfo

	temporaryDials sync.Map // map of temporary connections; peerID -> bool

	bootnodes *bootnodesWrapper // reference of all bootnodes for the node
}

// NewServer returns a new instance of the networking server
func NewServer(logger hclog.Logger, config *Config) (*Server, error) {
	logger = logger.Named("network")

	key, err := setupLibp2pKey(config.SecretsManager)
	if err != nil {
		return nil, err
	}

	listenAddr, err := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/%s/tcp/%d", config.Addr.IP.String(), config.Addr.Port))
	if err != nil {
		return nil, err
	}

	addrsFactory := func(addrs []multiaddr.Multiaddr) []multiaddr.Multiaddr {
		if config.NatAddr != nil {
			addr, _ := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/%s/tcp/%d", config.NatAddr.String(), config.Addr.Port))

			if addr != nil {
				addrs = []multiaddr.Multiaddr{addr}
			}
		} else if config.DNS != nil {
			addrs = []multiaddr.Multiaddr{config.DNS}
		}

		return addrs
	}

	host, err := libp2p.New(
		// Use noise as the encryption protocol
		libp2p.Security(noise.ID, noise.New),
		libp2p.ListenAddrs(listenAddr),
		libp2p.AddrsFactory(addrsFactory),
		libp2p.Identity(key),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create libp2p stack: %w", err)
	}

	emitter, err := host.EventBus().Emitter(new(peerEvent.PeerEvent))
	if err != nil {
		return nil, err
	}

	srv := &Server{
		logger:           logger,
		config:           config,
		host:             host,
		addrs:            host.Addrs(),
		peers:            make(map[peer.ID]*PeerConnInfo),
		dialQueue:        dial.NewDialQueue(),
		closeCh:          make(chan struct{}),
		emitterPeerEvent: emitter,
		protocols:        map[string]Protocol{},
		secretsManager:   config.SecretsManager,
		bootnodes: &bootnodesWrapper{
			bootnodeArr:       make([]*peer.AddrInfo, 0),
			bootnodesMap:      make(map[peer.ID]*peer.AddrInfo),
			bootnodeConnCount: 0,
		},
		connectionCounts: NewBlankConnectionInfo(
			config.MaxInboundPeers,
			config.MaxOutboundPeers,
		),
	}

	// start gossip protocol
	ps, err := pubsub.NewGossipSub(
		context.Background(),
		host, pubsub.WithPeerOutboundQueueSize(peerOutboundBufferSize),
		pubsub.WithValidateQueueSize(validateBufferSize),
	)
	if err != nil {
		return nil, err
	}

	srv.ps = ps

	return srv, nil
}

// HasFreeConnectionSlot checks if there are free connection slots in the specified direction [Thread safe]
func (s *Server) HasFreeConnectionSlot(direction network.Direction) bool {
	return s.connectionCounts.HasFreeConnectionSlot(direction)
}

// PeerConnInfo holds the connection information about the peer
type PeerConnInfo struct {
	Info peer.AddrInfo

	connDirections  map[network.Direction]bool
	protocolStreams map[string]*rawGrpc.ClientConn
}

// addProtocolStream adds a protocol stream
func (pci *PeerConnInfo) addProtocolStream(protocol string, stream *rawGrpc.ClientConn) {
	pci.protocolStreams[protocol] = stream
}

// removeProtocolStream removes and closes a protocol stream
func (pci *PeerConnInfo) removeProtocolStream(protocol string) error {
	stream, ok := pci.protocolStreams[protocol]
	if !ok {
		return nil
	}

	delete(pci.protocolStreams, protocol)

	if stream != nil {
		return stream.Close()
	}

	return nil
}

// getProtocolStream fetches the protocol stream, if any
func (pci *PeerConnInfo) getProtocolStream(protocol string) *rawGrpc.ClientConn {
	return pci.protocolStreams[protocol]
}

// setupLibp2pKey is a helper method for setting up the networking private key
func setupLibp2pKey(secretsManager secrets.SecretsManager) (crypto.PrivKey, error) {
	var key crypto.PrivKey

	if secretsManager.HasSecret(secrets.NetworkKey) {
		// The key is present in the secrets manager, read it
		networkingKey, readErr := ReadLibp2pKey(secretsManager)
		if readErr != nil {
			return nil, fmt.Errorf("unable to read networking private key from Secrets Manager, %w", readErr)
		}

		key = networkingKey
	} else {
		// The key is not present in the secrets manager, generate it
		libp2pKey, libp2pKeyEncoded, keyErr := GenerateAndEncodeLibp2pKey()
		if keyErr != nil {
			return nil, fmt.Errorf("unable to generate networking private key for Secrets Manager, %w", keyErr)
		}

		// Write the networking private key to disk
		if setErr := secretsManager.SetSecret(secrets.NetworkKey, libp2pKeyEncoded); setErr != nil {
			return nil, fmt.Errorf("unable to store networking private key to Secrets Manager, %w", setErr)
		}

		key = libp2pKey
	}

	return key, nil
}

// Start starts the networking services
func (s *Server) Start() error {
	addr, err := common.AddrInfoToString(s.AddrInfo())
	if err != nil {
		return err
	}

	s.logger.Info("LibP2P server running", "addr", addr)

	if setupErr := s.setupIdentity(); setupErr != nil {
		return fmt.Errorf("unable to setup identity, %w", setupErr)
	}

	// Set up the peer discovery mechanism if needed
	if !s.config.NoDiscover {
		// Parse the bootnode data
		if setupErr := s.setupBootnodes(); setupErr != nil {
			return fmt.Errorf("unable to parse bootnode data, %w", setupErr)
		}

		// Setup and start the discovery service
		if setupErr := s.setupDiscovery(); setupErr != nil {
			return fmt.Errorf("unable to setup discovery, %w", setupErr)
		}
	}

	go s.runDial()
	go s.keepAliveMinimumPeerConnections()

	// watch for disconnected peers
	s.host.Network().Notify(&network.NotifyBundle{
		DisconnectedF: func(net network.Network, conn network.Conn) {
			// Update the local connection metrics
			s.removePeer(conn.RemotePeer())
		},
	})

	return nil
}

// setupBootnodes sets up the node's bootnode connections
func (s *Server) setupBootnodes() error {
	// Check the bootnode config is present
	if s.config.Chain.Bootnodes == nil {
		return ErrNoBootnodes
	}

	// Check if at least one bootnode is specified
	if len(s.config.Chain.Bootnodes) < MinimumBootNodes {
		return ErrMinBootnodes
	}

	bootnodesArr := make([]*peer.AddrInfo, 0)
	bootnodesMap := make(map[peer.ID]*peer.AddrInfo)

	for _, rawAddr := range s.config.Chain.Bootnodes {
		bootnode, err := common.StringToAddrInfo(rawAddr)
		if err != nil {
			return fmt.Errorf("failed to parse bootnode %s: %w", rawAddr, err)
		}

		if bootnode.ID == s.host.ID() {
			s.logger.Info("Omitting bootnode with same ID as host", "id", bootnode.ID)

			continue
		}

		bootnodesArr = append(bootnodesArr, bootnode)
		bootnodesMap[bootnode.ID] = bootnode
	}

	// It's fine for the bootnodes field to be unprotected
	// at this point because it is initialized once (doesn't change),
	// and used only after this point
	s.bootnodes = &bootnodesWrapper{
		bootnodeArr:       bootnodesArr,
		bootnodesMap:      bootnodesMap,
		bootnodeConnCount: 0,
	}

	return nil
}

// keepAliveMinimumPeerConnections will attempt to make new connections
// if the active peer count is lesser than the specified limit.
func (s *Server) keepAliveMinimumPeerConnections() {
	for {
		select {
		case <-time.After(10 * time.Second):
		case <-s.closeCh:
			return
		}

		if s.numPeers() < MinimumPeerConnections {
			if s.config.NoDiscover || !s.bootnodes.hasBootnodes() {
				// dial unconnected peer
				randPeer := s.GetRandomPeer()
				if randPeer != nil && !s.IsConnected(*randPeer) {
					s.addToDialQueue(s.GetPeerInfo(*randPeer), common.PriorityRandomDial)
				}
			} else {
				// dial random unconnected bootnode
				if randomNode := s.GetRandomBootnode(); randomNode != nil {
					s.addToDialQueue(randomNode, common.PriorityRandomDial)
				}
			}
		}
	}
}

// runDial starts the networking server's dial loop.
// Essentially, the networking server monitors for any open connection slots
// and attempts to fill them as soon as they open up
func (s *Server) runDial() {
	slots := NewSlots(s.connectionCounts.maxOutboundConnectionCount)
	ctx, cancel := context.WithCancel(context.Background())

	defer cancel()

	if err := s.Subscribe(ctx, func(event *peerEvent.PeerEvent) {
		// Return back slot on PeerFailedToConnect or PeerDisconnected
		switch event.Type {
		case
			peerEvent.PeerFailedToConnect,
			peerEvent.PeerDisconnected:
			slots.Release()
			s.logger.Debug("slot released", "event", event.Type, "peerID", event.PeerID)
		}
	}); err != nil {
		s.logger.Error(
			"Cannot instantiate an event subscription for the dial manager",
			"err",
			err,
		)

		// Failing to subscribe to network events is fatal since the
		// dial manager relies on the event subscription routine to function
		return
	}

	for {
		if closed := s.dialQueue.Wait(ctx); closed {
			// The dial queue is closed, no further dial tasks are incoming
			return
		}

		for {
			tt := s.dialQueue.PopTask()
			if tt == nil {
				break
			}

			peerInfo := tt.GetAddrInfo()

			if s.IsConnected(peerInfo.ID) {
				continue
			}

			s.logger.Debug("Waiting for a dialing slot", "addr", peerInfo, "local", s.host.ID())

			if closed := slots.Take(ctx); closed {
				return
			}

			// the connection process is async because it involves connection (here) +
			// the handshake done in the identity service.
			go func() {
				s.logger.Debug("Dialing peer", "addr", peerInfo, "local", s.host.ID())

				if err := s.host.Connect(ctx, *peerInfo); err != nil {
					s.logger.Debug("failed to dial", "addr", peerInfo, "err", err.Error())

					s.emitEvent(peerInfo.ID, peerEvent.PeerFailedToConnect)
				}
			}()
		}
	}
}

// numPeers returns the number of connected peers [Thread safe]
func (s *Server) numPeers() int64 {
	s.peersLock.Lock()
	defer s.peersLock.Unlock()

	return int64(len(s.peers))
}

// Peers returns a copy of the networking server's peer connection info set.
// Only one (initial) connection (inbound OR outbound) per peer is contained [Thread safe]
func (s *Server) Peers() []*PeerConnInfo {
	s.peersLock.Lock()
	defer s.peersLock.Unlock()

	peers := make([]*PeerConnInfo, 0)
	for _, connectionInfo := range s.peers {
		peers = append(peers, connectionInfo)
	}

	return peers
}

// hasPeer checks if the peer is present in the peers list [Thread safe]
func (s *Server) hasPeer(peerID peer.ID) bool {
	s.peersLock.Lock()
	defer s.peersLock.Unlock()

	_, ok := s.peers[peerID]

	return ok
}

// IsConnected checks if the networking server is connected to a peer
func (s *Server) IsConnected(peerID peer.ID) bool {
	return s.host.Network().Connectedness(peerID) == network.Connected
}

// GetProtocols fetches the list of node-supported protocols
func (s *Server) GetProtocols(peerID peer.ID) ([]string, error) {
	protocols, err := s.host.Peerstore().GetProtocols(peerID)
	if err != nil {
		return nil, err
	}

	return protocol.ConvertToStrings(protocols), nil
}

// removePeer removes a peer from the networking server's peer list,
// and updates relevant counters and metrics. It is called from the
// disconnection callback of the libp2p network bundle (when the connection is closed)
func (s *Server) removePeer(peerID peer.ID) {
	s.logger.Info("Peer disconnected", "id", peerID)

	// Remove the peer from the peers map
	connectionInfo := s.removePeerInfo(peerID)
	if connectionInfo == nil {
		// The peer wasn't present in the local peers info table
		// so no action should be taken further
		return
	}

	// Emit the event alerting listeners
	s.emitEvent(peerID, peerEvent.PeerDisconnected)
}

// removePeerInfo removes (pops) peer connection info from the networking
// server's peer map. Returns nil if no peer was removed
func (s *Server) removePeerInfo(peerID peer.ID) *PeerConnInfo {
	s.peersLock.Lock()
	defer s.peersLock.Unlock()

	// Remove the peer from the peers map
	connectionInfo, ok := s.peers[peerID]
	if !ok {
		// Peer is not present in the peers map
		s.logger.Warn(
			fmt.Sprintf("Attempted removing missing peer info %s", peerID),
		)

		return nil
	}

	// Delete the peer from the peers map
	delete(s.peers, peerID)

	// Update connection counters
	for connDirection, active := range connectionInfo.connDirections {
		if active {
			s.connectionCounts.UpdateConnCountByDirection(-1, connDirection)
			s.updateConnCountMetrics(connDirection)
			s.updateBootnodeConnCount(peerID, -1)
		}
	}

	metrics.SetGauge([]string{networkMetrics, "peers"}, float32(len(s.peers)))

	return connectionInfo
}

// updateBootnodeConnCount attempts to update the bootnode connection count
// by delta if the action is valid [Thread safe]
func (s *Server) updateBootnodeConnCount(peerID peer.ID, delta int64) {
	if s.config.NoDiscover || !s.bootnodes.isBootnode(peerID) {
		// If the discovery service is not running
		// or the peer is not a bootnode, there is no need
		// to update bootnode connection counters
		return
	}

	s.bootnodes.increaseBootnodeConnCount(delta)
}

// DisconnectFromPeer disconnects the networking server from the specified peer
func (s *Server) DisconnectFromPeer(peer peer.ID, reason string) {
	if s.host.Network().Connectedness(peer) == network.Connected {
		s.logger.Info("Closing connection", "id", peer, "reason", reason)

		if err := s.host.Network().ClosePeer(peer); err != nil {
			s.logger.Error("Unable to gracefully close connection", "id", peer, "err", err)
		}
	}
}

var (
	// Anything below 35s is prone to false timeouts, as seen from empirical test data
	DefaultJoinTimeout   = 100 * time.Second
	DefaultBufferTimeout = DefaultJoinTimeout + time.Second*5
)

// JoinPeer attempts to add a new peer to the networking server
func (s *Server) JoinPeer(rawPeerMultiaddr string) error {
	// Parse the raw string to a MultiAddr format
	parsedMultiaddr, err := multiaddr.NewMultiaddr(rawPeerMultiaddr)
	if err != nil {
		return err
	}

	// Extract the peer info from the Multiaddr
	peerInfo, err := peer.AddrInfoFromP2pAddr(parsedMultiaddr)
	if err != nil {
		return err
	}

	// Mark the peer as ripe for dialing (async)
	s.joinPeer(peerInfo)

	return nil
}

// joinPeer creates a new dial task for the peer (for async joining)
func (s *Server) joinPeer(peerInfo *peer.AddrInfo) {
	s.logger.Info("Join request", "addr", peerInfo)

	// This method can be completely refactored to support some kind of active
	// feedback information on the dial status, and not just asynchronous updates.
	// For this feature to work, the networking server requires a flexible event subscription
	// manager that is configurable and cancelable at any point in time
	s.addToDialQueue(peerInfo, common.PriorityRequestedDial)
}

func (s *Server) Close() error {
	err := s.host.Close()
	s.dialQueue.Close()

	if !s.config.NoDiscover {
		s.discovery.Close()
	}

	close(s.closeCh)

	return err
}

// NewProtoConnection opens up a new stream on the set protocol to the peer,
// and returns a reference to the connection
func (s *Server) NewProtoConnection(protocol string, peerID peer.ID) (*rawGrpc.ClientConn, error) {
	s.protocolsLock.Lock()
	defer s.protocolsLock.Unlock()

	p, ok := s.protocols[protocol]
	if !ok {
		return nil, fmt.Errorf("protocol not found: %s", protocol)
	}

	stream, err := s.NewStream(protocol, peerID)
	if err != nil {
		return nil, err
	}

	return p.Client(stream)
}

func (s *Server) NewStream(proto string, id peer.ID) (network.Stream, error) {
	return s.host.NewStream(context.Background(), id, protocol.ID(proto))
}

type Protocol interface {
	Client(network.Stream) (*rawGrpc.ClientConn, error)
	Handler() func(network.Stream)
}

func (s *Server) RegisterProtocol(id string, p Protocol) {
	s.protocolsLock.Lock()
	defer s.protocolsLock.Unlock()

	s.protocols[id] = p
	s.wrapStream(id, p.Handler())
}

func (s *Server) wrapStream(id string, handle func(network.Stream)) {
	s.host.SetStreamHandler(protocol.ID(id), func(stream network.Stream) {
		peerID := stream.Conn().RemotePeer()
		s.logger.Debug("open stream", "protocol", id, "peer", peerID)

		handle(stream)
	})
}

func (s *Server) AddrInfo() *peer.AddrInfo {
	return &peer.AddrInfo{
		ID:    s.host.ID(),
		Addrs: s.addrs,
	}
}

func (s *Server) addToDialQueue(addr *peer.AddrInfo, priority common.DialPriority) {
	s.dialQueue.AddTask(addr, priority)
	s.emitEvent(addr.ID, peerEvent.PeerAddedToDialQueue)
}

func (s *Server) emitEvent(peerID peer.ID, peerEventType peerEvent.PeerEventType) {
	// POTENTIALLY BLOCKING
	if err := s.emitterPeerEvent.Emit(peerEvent.PeerEvent{
		PeerID: peerID,
		Type:   peerEventType,
	}); err != nil {
		s.logger.Info("failed to emit event", "peer", peerID, "type", peerEventType, "err", err)
	}
}

// Subscribe is a helper method to run subscription of PeerEvents
func (s *Server) Subscribe(ctx context.Context, handler func(evnt *peerEvent.PeerEvent)) error {
	sub, err := s.host.EventBus().Subscribe(new(peerEvent.PeerEvent))
	if err != nil {
		return err
	}

	go func() {
		defer sub.Close()

		for {
			select {
			case <-ctx.Done():
				return

			case <-s.closeCh:
				return

			case evnt := <-sub.Out():
				if obj, ok := evnt.(peerEvent.PeerEvent); ok {
					handler(&obj)
				}
			}
		}
	}()

	return nil
}

// SubscribeCh returns an event of of subscription events
func (s *Server) SubscribeCh(ctx context.Context) (<-chan *peerEvent.PeerEvent, error) {
	ch := make(chan *peerEvent.PeerEvent)
	ctx, cancel := context.WithCancel(ctx)

	err := s.Subscribe(ctx, func(evnt *peerEvent.PeerEvent) {
		select {
		case <-ctx.Done():
			return
		case ch <- evnt:
		}
	})

	cleanup := func() {
		cancel()
		close(ch)
	}

	if err != nil {
		cleanup()

		return nil, err
	}

	go func() {
		<-s.closeCh

		cleanup()
	}()

	return ch, nil
}

// updateConnCountMetrics updates the connection count metrics
func (s *Server) updateConnCountMetrics(direction network.Direction) {
	switch direction {
	case network.DirInbound:
		metrics.SetGauge([]string{networkMetrics, "inbound_connections_count"},
			float32(s.connectionCounts.GetInboundConnCount()))

	case network.DirOutbound:
		metrics.SetGauge([]string{networkMetrics, "outbound_connections_count"},
			float32(s.connectionCounts.GetOutboundConnCount()))
	}
}

// updatePendingConnCountMetrics updates the pending connection count metrics
func (s *Server) updatePendingConnCountMetrics(direction network.Direction) {
	switch direction {
	case network.DirInbound:
		metrics.SetGauge([]string{networkMetrics, "pending_inbound_connections_count"},
			float32(s.connectionCounts.GetPendingInboundConnCount()))

	case network.DirOutbound:
		metrics.SetGauge([]string{networkMetrics, "pending_outbound_connections_count"},
			float32(s.connectionCounts.GetPendingOutboundConnCount()))
	}
}
