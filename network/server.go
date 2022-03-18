package network

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"github.com/0xPolygon/polygon-edge/network/common"
	"github.com/0xPolygon/polygon-edge/network/connections"
	"github.com/0xPolygon/polygon-edge/network/dial"
	"github.com/0xPolygon/polygon-edge/network/discovery"
	"github.com/0xPolygon/polygon-edge/network/grpc"
	"github.com/0xPolygon/polygon-edge/network/identity"
	"github.com/0xPolygon/polygon-edge/network/proto"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/peerstore"
	kb "github.com/libp2p/go-libp2p-kbucket"
	noise "github.com/libp2p/go-libp2p-noise"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	peerEvent "github.com/0xPolygon/polygon-edge/network/event"
	"github.com/0xPolygon/polygon-edge/secrets"
	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/event"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
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
)

const (
	defaultBucketSize = 20
	DefaultDialRatio  = 0.2

	DefaultLibp2pPort int = 1478

	MinimumPeerConnections int64 = 1
	MinimumBootNodes       int   = 1
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

	peers     map[peer.ID]*Peer // map of all peer connections
	peersLock sync.Mutex        // lock for the peer map

	metrics *Metrics // reference for metrics tracking

	dialQueue *dial.DialQueue // queue used to asynchronously connect to peers

	identity  *identity.IdentityService   // service used for handshaking with peers
	discovery *discovery.DiscoveryService // service used for discovering other peers

	protocols     map[string]Protocol // supported protocols
	protocolsLock sync.Mutex          // lock for the supported protocols map

	secretsManager secrets.SecretsManager // secrets manager for networking keys

	ps *pubsub.PubSub // reference to the networking PubSub service

	joinWatchers     map[peer.ID]chan error // set of networking event watchers
	joinWatchersLock sync.Mutex             // lock for the networking event watchers map

	emitterPeerEvent event.Emitter // event emitter for listeners

	connectionCounts *connections.ConnectionInfo

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
		peers:            map[peer.ID]*Peer{},
		metrics:          config.Metrics,
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
		connectionCounts: connections.NewBlankConnectionInfo(
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

// EmitEvent emits a specified event to the networking server's event bus
func (s *Server) EmitEvent(event *peerEvent.PeerEvent) {
	s.emitEvent(event.PeerID, event.Type)
}

// IsTemporaryDial checks if a peer connection is temporary [Thread safe]
func (s *Server) IsTemporaryDial(peerID peer.ID) bool {
	_, ok := s.temporaryDials.Load(peerID)

	return ok
}

// IsBootnode checks if a peer is a bootnode [Thread safe]
func (s *Server) IsBootnode(peerID peer.ID) bool {
	return s.bootnodes.isBootnode(peerID)
}

// GetBootnodeConnCount fetches the number of active bootnode connections [Thread safe]
func (s *Server) GetBootnodeConnCount() int64 {
	return s.bootnodes.getBootnodeConnCount()
}

// FetchAndSetTemporaryDial loads the temporary status of a peer connection, and
// sets a new value [Thread safe]
func (s *Server) FetchAndSetTemporaryDial(peerID peer.ID, newValue bool) bool {
	_, loaded := s.temporaryDials.LoadOrStore(peerID, newValue)

	return loaded
}

// RemoveTemporaryDial removes a peer connection as temporary [Thread safe]
func (s *Server) RemoveTemporaryDial(peerID peer.ID) {
	s.temporaryDials.Delete(peerID)
}

// HasFreeConnectionSlot checks if there are free connection slots in the specified direction [Thread safe]
func (s *Server) HasFreeConnectionSlot(direction network.Direction) bool {
	return s.connectionCounts.HasFreeConnectionSlot(direction)
}

type Peer struct {
	srv *Server

	Info peer.AddrInfo

	connDirection network.Direction
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
	s.logger.Info("LibP2P server running", "addr", common.AddrInfoToString(s.AddrInfo()))

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
	go s.checkPeerConnections()

	go func() {
		if err := s.runJoinWatcher(); err != nil {
			s.logger.Error(fmt.Sprintf("Unable to start join watcher service, %v", err))
		}
	}()

	// watch for disconnected peers
	s.host.Network().Notify(&network.NotifyBundle{
		DisconnectedF: func(net network.Network, conn network.Conn) {
			go func() {
				s.removePeer(conn.RemotePeer())
			}()
		},
	})

	return nil
}

// setupDiscovery Sets up the discovery service for the node
func (s *Server) setupDiscovery() error {
	// Set up a fresh routing table
	keyID := kb.ConvertPeerID(s.host.ID())

	routingTable, err := kb.NewRoutingTable(
		defaultBucketSize,
		keyID,
		time.Minute,
		s.host.Peerstore(),
		10*time.Second,
		nil,
	)
	if err != nil {
		return err
	}

	// Set the PeerAdded event handler
	routingTable.PeerAdded = func(p peer.ID) {
		info := s.host.Peerstore().PeerInfo(p)
		s.addToDialQueue(&info, common.PriorityRandomDial)
	}

	// Set the PeerRemoved event handler
	routingTable.PeerRemoved = func(p peer.ID) {
		s.dialQueue.DeleteTask(p)
	}

	// Create an instance of the discovery service
	discoveryService := discovery.NewDiscoveryService(
		s,
		routingTable,
		s.logger,
		make(chan struct{}),
	)

	// Register a network event handler
	if subscribeErr := s.SubscribeFn(discoveryService.HandleNetworkEvent); subscribeErr != nil {
		return fmt.Errorf("unable to subscribe to network events, %w", subscribeErr)
	}

	// Register the actual discovery service as a valid protocol
	s.registerDiscoveryService(discoveryService)

	// Make sure the discovery service has the bootnodes in its routing table,
	// and instantiates connections to them
	discoveryService.ConnectToBootnodes(s.bootnodes.getBootnodes())

	// Start the discovery service
	discoveryService.Start()

	// Set the discovery service reference
	s.discovery = discoveryService

	return nil
}

// registerDiscoveryService registers the discovery protocol to be available
func (s *Server) registerDiscoveryService(discovery *discovery.DiscoveryService) {
	grpcStream := grpc.NewGrpcStream()
	proto.RegisterDiscoveryServer(grpcStream.GrpcServer(), discovery)
	grpcStream.Serve()

	s.RegisterProtocol(common.DiscProto, grpcStream)
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
		bootnodeConnCount: int64(len(bootnodesArr)),
	}

	return nil
}

// setupIdentity sets up the identity service for the node
func (s *Server) setupIdentity() error {
	// Create an instance of the identity service
	identityService := identity.NewIdentityService(
		s,
		s.logger,
		int64(s.config.Chain.Params.ChainID),
		s.host.ID(),
	)

	// Register the identity service protocol
	s.registerIdentityService(identityService)

	// Register the network notify bundle handlers
	s.host.Network().Notify(identityService.GetNotifyBundle())

	// Set the identity service
	s.identity = identityService

	return nil
}

// registerIdentityService registers the identity service
func (s *Server) registerIdentityService(identityService *identity.IdentityService) {
	grpcStream := grpc.NewGrpcStream()
	proto.RegisterIdentityServer(grpcStream.GrpcServer(), identityService)
	grpcStream.Serve()

	s.RegisterProtocol(common.IdentityProto, grpcStream)
}

// AddToPeerStore adds peer information to the node's peer store
func (s *Server) AddToPeerStore(peerInfo *peer.AddrInfo) {
	s.host.Peerstore().AddAddr(peerInfo.ID, peerInfo.Addrs[0], peerstore.AddressTTL)
}

// checkPeerCount will attempt to make new connections if the active peer count is lesser than the specified limit.
func (s *Server) checkPeerConnections() {
	for {
		select {
		case <-time.After(10 * time.Second):
		case <-s.closeCh:
			return
		}

		if s.numPeers() < MinimumPeerConnections {
			if s.config.NoDiscover || !s.bootnodes.hasBootnodes() {
				//TODO: dial peers from the peerstore
			} else {
				randomNode := s.GetRandomBootnode()
				s.addToDialQueue(randomNode, common.PriorityRandomDial)
			}
		}
	}
}

// runDial starts the networking server's dial loop.
// Essentially, the networking server monitors for any open connection slots
// and attempts to fill them as soon as they open up
func (s *Server) runDial() {
	// watch for events of peers included or removed
	notifyCh := make(chan struct{})
	err := s.SubscribeFn(func(event *peerEvent.PeerEvent) {
		// Only concerned about the listed event types
		switch event.Type {
		case
			peerEvent.PeerConnected,
			peerEvent.PeerFailedToConnect,
			peerEvent.PeerDisconnected,
			peerEvent.PeerDialCompleted,
			peerEvent.PeerAddedToDialQueue:
		default:
			return
		}

		select {
		case notifyCh <- struct{}{}:
		default:
		}
	})

	if err != nil {
		s.logger.Error("dial manager failed to subscribe", "err", err)
	}

	for {
		// TODO: Right now the dial task are done sequentially because Connect
		// is a blocking request. In the future we should try to make up to
		// maxDials requests concurrently
		for s.connectionCounts.HasFreeOutboundConn() {
			tt := s.dialQueue.PopTask()
			if tt == nil {
				// dial closed
				return
			}

			taskInfo := tt.GetTaskInfo()

			s.logger.Debug("dial", "local", s.host.ID(), "addr", taskInfo.String())

			if s.isConnected(taskInfo.ID) {
				// the node is already connected, send an event to wake up
				// any join watchers
				s.emitEvent(taskInfo.ID, peerEvent.PeerAlreadyConnected)
			} else {
				// the connection process is async because it involves connection (here) +
				// the handshake done in the identity service.
				if err := s.host.Connect(context.Background(), *taskInfo); err != nil {
					s.logger.Debug("failed to dial", "addr", taskInfo.String(), "err", err)

					s.emitEvent(taskInfo.ID, peerEvent.PeerFailedToConnect)
				}
			}
		}

		// wait until there is a change in the state of a peer that
		// might involve a new dial slot available
		select {
		case <-notifyCh:
		case <-s.closeCh:
			return
		}
	}
}

// numPeers returns the number of connected peers [Thread safe]
func (s *Server) numPeers() int64 {
	s.peersLock.Lock()
	defer s.peersLock.Unlock()

	return int64(len(s.peers))
}

// GetRandomBootnode fetches a random bootnode that's currently
// NOT connected, if any
func (s *Server) GetRandomBootnode() *peer.AddrInfo {
	nonConnectedNodes := make([]*peer.AddrInfo, 0)

	for _, v := range s.bootnodes.getBootnodes() {
		if !s.hasPeer(v.ID) {
			nonConnectedNodes = append(nonConnectedNodes, v)
		}
	}

	if len(nonConnectedNodes) > 0 {
		randNum, _ := rand.Int(rand.Reader, big.NewInt(int64(len(nonConnectedNodes))))

		return nonConnectedNodes[randNum.Int64()]
	}

	return nil
}

// Peers returns a copy of the networking server's peer set [Thread safe]
func (s *Server) Peers() []*Peer {
	s.peersLock.Lock()
	defer s.peersLock.Unlock()

	peers := make([]*Peer, 0, len(s.peers))
	for _, p := range s.peers {
		peers = append(peers, p)
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

// isConnected checks if the networking server is connected to a peer
func (s *Server) isConnected(peerID peer.ID) bool {
	return s.host.Network().Connectedness(peerID) == network.Connected
}

// GetProtocols fetches the list of node-supported protocols
func (s *Server) GetProtocols(peerID peer.ID) ([]string, error) {
	return s.host.Peerstore().GetProtocols(peerID)
}

// GetPeerInfo fetches the information of a peer
func (s *Server) GetPeerInfo(peerID peer.ID) *peer.AddrInfo {
	info := s.host.Peerstore().PeerInfo(peerID)

	return &info
}

// AddPeer adds a new peer to the networking server's peer list,
// and updates relevant counters and metrics
func (s *Server) AddPeer(id peer.ID, direction network.Direction) {
	s.peersLock.Lock()

	s.logger.Info("Peer connected", "id", id.String())

	s.peers[id] = &Peer{
		srv:           s,
		Info:          s.host.Peerstore().PeerInfo(id),
		connDirection: direction,
	}

	// Update connection counters
	s.connectionCounts.UpdateConnCountByDirection(1, direction)
	s.updateConnCountMetrics(direction)
	s.updateBootnodeConnCount(id, 1)

	// Update the metric stats
	s.metrics.TotalPeerCount.Set(float64(len(s.peers)))

	s.peersLock.Unlock()

	// Emit the event alerting listeners
	s.emitEvent(id, peerEvent.PeerConnected)
}

// removePeer removes a peer from the networking server's peer list,
// and updates relevant counters and metrics
func (s *Server) removePeer(id peer.ID) {
	s.peersLock.Lock()

	s.logger.Info("Peer disconnected", "id", id.String())

	// Remove the peer from the peers map
	if peer, ok := s.peers[id]; ok {
		// Update connection counters
		s.connectionCounts.UpdateConnCountByDirection(-1, peer.connDirection)
		s.updateConnCountMetrics(peer.connDirection)
		s.updateBootnodeConnCount(id, -1)

		delete(s.peers, id)
	}

	// Close network connections to the peer
	if closeErr := s.host.Network().ClosePeer(id); closeErr != nil {
		s.logger.Error(
			fmt.Sprintf("Unable to gracefully close connection to peer [%s], %v", id.String(), closeErr),
		)
	}

	s.metrics.TotalPeerCount.Set(float64(len(s.peers)))

	s.peersLock.Unlock()

	// Emit the event alerting listeners
	s.emitEvent(id, peerEvent.PeerDisconnected)
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

// UpdatePendingConnCount updates the pending connection count in the specified direction [Thread safe]
func (s *Server) UpdatePendingConnCount(delta int64, direction network.Direction) {
	s.connectionCounts.UpdatePendingConnCountByDirection(delta, direction)

	s.updatePendingConnCountMetrics(direction)
}

// DisconnectFromPeer disconnects the networking server from the specified peer
func (s *Server) DisconnectFromPeer(peer peer.ID, reason string) {
	if s.host.Network().Connectedness(peer) == network.Connected {
		s.logger.Info(fmt.Sprintf("Closing connection to peer [%s] for reason [%s]", peer.String(), reason))

		if closeErr := s.host.Network().ClosePeer(peer); closeErr != nil {
			s.logger.Error(fmt.Sprintf("Unable to gracefully close peer connection, %v", closeErr))
		}
	}
}

var (
	// Anything below 35s is prone to false timeouts, as seen from empirical test data
	DefaultJoinTimeout   = 40 * time.Second
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
	s.logger.Info("Join request", "addr", peerInfo.String())

	// This method can be completely refactored to support some kind of active
	// feedback information on the dial status, and not just asynchronous updates.
	// For this feature to work, the networking server requires a flexible event subscription
	// manager that is configurable and cancelable at any point in time
	s.addToDialQueue(peerInfo, common.PriorityRequestedDial)
}

func (s *Server) watch(peerID peer.ID, dur time.Duration) error {
	ch := make(chan error)

	s.joinWatchersLock.Lock()
	if s.joinWatchers == nil {
		s.joinWatchers = map[peer.ID]chan error{}
	}

	s.joinWatchers[peerID] = ch
	s.joinWatchersLock.Unlock()

	select {
	case <-time.After(dur):
		s.joinWatchersLock.Lock()
		delete(s.joinWatchers, peerID)
		s.joinWatchersLock.Unlock()

		return fmt.Errorf("timeout %s %s", s.host.ID(), peerID)
	case err := <-ch:
		return err
	}
}

func (s *Server) runJoinWatcher() error {
	return s.SubscribeFn(func(event *peerEvent.PeerEvent) {
		switch event.Type {
		case
			peerEvent.PeerConnected,
			peerEvent.PeerFailedToConnect,
			peerEvent.PeerAlreadyConnected:
		default:
			return
		}

		// try to find a watcher for this peer
		s.joinWatchersLock.Lock()
		errCh, ok := s.joinWatchers[event.PeerID]
		if ok {
			errCh <- nil
			delete(s.joinWatchers, event.PeerID)
		}
		s.joinWatchersLock.Unlock()
	})
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

func (s *Server) NewProtoStream(proto string, id peer.ID) (interface{}, error) {
	s.protocolsLock.Lock()
	defer s.protocolsLock.Unlock()

	p, ok := s.protocols[proto]
	if !ok {
		return nil, fmt.Errorf("protocol not found: %s", proto)
	}

	stream, err := s.NewStream(proto, id)
	if err != nil {
		return nil, err
	}

	return p.Client(stream), nil
}

func (s *Server) NewStream(proto string, id peer.ID) (network.Stream, error) {
	return s.host.NewStream(context.Background(), id, protocol.ID(proto))
}

type Protocol interface {
	Client(network.Stream) interface{}
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
	evnt := peerEvent.PeerEvent{
		PeerID: peerID,
		Type:   peerEventType,
	}

	if err := s.emitterPeerEvent.Emit(evnt); err != nil {
		s.logger.Info("failed to emit event", "peer", evnt.PeerID, "type", evnt.Type, "err", err)
	}
}

type Subscription struct {
	sub event.Subscription
	ch  chan *peerEvent.PeerEvent
}

func (s *Subscription) run() {
	// convert interface{} to *PeerEvent channels
	for {
		evnt := <-s.sub.Out()
		if obj, ok := evnt.(peerEvent.PeerEvent); ok {
			s.ch <- &obj
		}
	}
}

func (s *Subscription) GetCh() chan *peerEvent.PeerEvent {
	return s.ch
}

func (s *Subscription) Get() *peerEvent.PeerEvent {
	obj := <-s.ch

	return obj
}

func (s *Subscription) Close() {
	s.sub.Close()
}

// Subscribe starts a PeerEvent subscription
func (s *Server) Subscribe() (*Subscription, error) {
	raw, err := s.host.EventBus().Subscribe(new(peerEvent.PeerEvent))
	if err != nil {
		return nil, err
	}

	sub := &Subscription{
		sub: raw,
		ch:  make(chan *peerEvent.PeerEvent),
	}
	go sub.run()

	return sub, nil
}

// SubscribeFn is a helper method to run subscription of PeerEvents
func (s *Server) SubscribeFn(handler func(evnt *peerEvent.PeerEvent)) error {
	sub, err := s.Subscribe()
	if err != nil {
		return err
	}

	go func() {
		for {
			select {
			case evnt := <-sub.GetCh():
				handler(evnt)

			case <-s.closeCh:
				sub.Close()

				return
			}
		}
	}()

	return nil
}

// SubscribeCh returns an event of of subscription events
func (s *Server) SubscribeCh() (<-chan *peerEvent.PeerEvent, error) {
	ch := make(chan *peerEvent.PeerEvent)

	var isClosed int32 = 0

	err := s.SubscribeFn(func(evnt *peerEvent.PeerEvent) {
		if atomic.LoadInt32(&isClosed) == 0 {
			ch <- evnt
		}
	})
	if err != nil {
		atomic.StoreInt32(&isClosed, 1)
		close(ch)

		return nil, err
	}

	go func() {
		<-s.closeCh
		atomic.StoreInt32(&isClosed, 1)
		close(ch)
	}()

	return ch, nil
}

// updateConnCountMetrics updates the connection count metrics
func (s *Server) updateConnCountMetrics(direction network.Direction) {
	switch direction {
	case network.DirInbound:
		s.metrics.InboundConnectionsCount.Set(
			float64(s.connectionCounts.GetInboundConnCount()),
		)
	case network.DirOutbound:
		s.metrics.OutboundConnectionsCount.Set(
			float64(s.connectionCounts.GetOutboundConnCount()),
		)
	}
}

// updatePendingConnCountMetrics updates the pending connection count metrics
func (s *Server) updatePendingConnCountMetrics(direction network.Direction) {
	switch direction {
	case network.DirInbound:
		s.metrics.PendingInboundConnectionsCount.Set(
			float64(s.connectionCounts.GetPendingInboundConnCount()),
		)
	case network.DirOutbound:
		s.metrics.PendingOutboundConnectionsCount.Set(
			float64(s.connectionCounts.GetPendingOutboundConnCount()),
		)
	}
}
