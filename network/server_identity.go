package network

import (
	"github.com/0xPolygon/polygon-edge/network/common"
	peerEvent "github.com/0xPolygon/polygon-edge/network/event"
	"github.com/0xPolygon/polygon-edge/network/grpc"
	"github.com/0xPolygon/polygon-edge/network/identity"
	"github.com/0xPolygon/polygon-edge/network/proto"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	rawGrpc "google.golang.org/grpc"
)

// NewIdentityClient returns a new identity service client connection
func (s *Server) NewIdentityClient(peerID peer.ID) (proto.IdentityClient, error) {
	// Create a new stream connection and return it
	protoStream, err := s.newProtoConnection(common.IdentityProto, peerID)
	if err != nil {
		return nil, err
	}

	// Identity protocol connections are temporary and not saved anywhere
	return proto.NewIdentityClient(protoStream), nil
}

// AddPeer adds a new peer to the networking server's peer list,
// and updates relevant counters and metrics
func (s *Server) AddPeer(id peer.ID, direction network.Direction) {
	s.logger.Info("Peer connected", "id", id.String())

	// Update the peer connection info
	if connectionExists := s.addPeerInfo(id, direction); connectionExists {
		// The peer connection information was already present in the networking
		// server, so no connection metrics should be updated further
		return
	}

	// Emit the event alerting listeners
	// WARNING: THIS CALL IS POTENTIALLY BLOCKING
	// UNDER HEAVY LOAD. IT SHOULD BE SUBSTITUTED
	// WITH AN EVENT SYSTEM THAT ACTUALLY WORKS
	s.emitEvent(id, peerEvent.PeerConnected)
}

// addPeerInfo updates the networking server's internal peer info table
// and returns a flag indicating if the same peer connection previously existed.
// In case the peer connection previously existed, this is a noop
func (s *Server) addPeerInfo(id peer.ID, direction network.Direction) bool {
	s.peersLock.Lock()
	defer s.peersLock.Unlock()

	connectionInfo, connectionExists := s.peers[id]
	if connectionExists && connectionInfo.connDirections[direction] {
		// Check if this peer already has an active connection status (saved info).
		// There is no need to do further processing
		return true
	}

	// Check if the connection info is already initialized
	if !connectionExists {
		// Create a new record for the connection info
		connectionInfo = &PeerConnInfo{
			Info:            s.host.Peerstore().PeerInfo(id),
			connDirections:  make(map[network.Direction]bool),
			protocolStreams: make(map[string]*rawGrpc.ClientConn),
		}
	}

	// Save the connection info to the networking server
	connectionInfo.connDirections[direction] = true

	s.peers[id] = connectionInfo

	// Update connection counters
	s.connectionCounts.UpdateConnCountByDirection(1, direction)
	s.updateConnCountMetrics(direction)
	s.updateBootnodeConnCount(id, 1)

	// Update the metric stats
	s.metrics.TotalPeerCount.Set(
		float64(len(s.peers)),
	)

	return false
}

// UpdatePendingConnCount updates the pending connection count in the specified direction [Thread safe]
func (s *Server) UpdatePendingConnCount(delta int64, direction network.Direction) {
	s.connectionCounts.UpdatePendingConnCountByDirection(delta, direction)

	s.updatePendingConnCountMetrics(direction)
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

	return nil
}

// registerIdentityService registers the identity service
func (s *Server) registerIdentityService(identityService *identity.IdentityService) {
	grpcStream := grpc.NewGrpcStream()
	proto.RegisterIdentityServer(grpcStream.GrpcServer(), identityService)
	grpcStream.Serve()

	s.RegisterProtocol(common.IdentityProto, grpcStream)
}
