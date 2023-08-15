package network

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"time"

	"github.com/0xPolygon/polygon-edge/network/common"
	"github.com/0xPolygon/polygon-edge/network/discovery"
	"github.com/0xPolygon/polygon-edge/network/grpc"
	"github.com/0xPolygon/polygon-edge/network/proto"
	kb "github.com/libp2p/go-libp2p-kbucket"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	rawGrpc "google.golang.org/grpc"
)

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

// GetBootnodeConnCount fetches the number of active bootnode connections [Thread safe]
func (s *Server) GetBootnodeConnCount() int64 {
	return s.bootnodes.getBootnodeConnCount()
}

// getProtoStream returns an active protocol stream if present, otherwise
// it returns nil
func (s *Server) getProtoStream(protocol string, peerID peer.ID) *rawGrpc.ClientConn {
	s.peersLock.Lock()
	defer s.peersLock.Unlock()

	connectionInfo, ok := s.peers[peerID]
	if !ok {
		return nil
	}

	return connectionInfo.getProtocolStream(protocol)
}

// NewDiscoveryClient returns a new or existing discovery service client connection
func (s *Server) NewDiscoveryClient(peerID peer.ID) (proto.DiscoveryClient, error) {
	// Temporary dials are never added to the peer store,
	// so they have a special status when doing discovery
	isTemporaryDial := s.IsTemporaryDial(peerID)

	// Check if there is a peer connection at this point in time,
	// as there might have been a disconnection previously
	if !s.IsConnected(peerID) && !isTemporaryDial {
		return nil, fmt.Errorf("could not initialize new discovery client - peer [%s] not connected",
			peerID.String())
	}

	// Check if there is an active stream connection already
	if protoStream := s.getProtoStream(common.DiscProto, peerID); protoStream != nil {
		return proto.NewDiscoveryClient(protoStream), nil
	}

	// Create a new stream connection and return it
	protoStream, err := s.NewProtoConnection(common.DiscProto, peerID)
	if err != nil {
		return nil, err
	}

	// Discovery protocol streams should be saved,
	// since they are referenced later on,
	// if they are not temporary
	if !isTemporaryDial {
		s.SaveProtocolStream(common.DiscProto, protoStream, peerID)
	}

	return proto.NewDiscoveryClient(protoStream), nil
}

// SaveProtocolStream saves the protocol stream to the peer
// protocol stream reference [Thread safe]
func (s *Server) SaveProtocolStream(
	protocol string,
	stream *rawGrpc.ClientConn,
	peerID peer.ID,
) {
	s.peersLock.Lock()
	defer s.peersLock.Unlock()

	connectionInfo, ok := s.peers[peerID]
	if !ok {
		s.logger.Warn(
			fmt.Sprintf(
				"Attempted to save protocol %s stream for non-existing peer %s",
				protocol,
				peerID,
			),
		)

		return
	}

	connectionInfo.addProtocolStream(protocol, stream)
}

// CloseProtocolStream closes a protocol stream to the specified peer
func (s *Server) CloseProtocolStream(protocol string, peerID peer.ID) error {
	s.peersLock.Lock()
	defer s.peersLock.Unlock()

	connectionInfo, ok := s.peers[peerID]
	if !ok {
		return nil
	}

	return connectionInfo.removeProtocolStream(protocol)
}

// AddToPeerStore adds peer information to the node's peer store
func (s *Server) AddToPeerStore(peerInfo *peer.AddrInfo) {
	s.host.Peerstore().AddAddr(peerInfo.ID, peerInfo.Addrs[0], peerstore.AddressTTL)
}

// RemoveFromPeerStore removes peer information from the node's peer store
func (s *Server) RemoveFromPeerStore(peerInfo *peer.AddrInfo) {
	s.host.Peerstore().RemovePeer(peerInfo.ID)
}

// GetPeerInfo fetches the information of a peer
func (s *Server) GetPeerInfo(peerID peer.ID) *peer.AddrInfo {
	info := s.host.Peerstore().PeerInfo(peerID)

	return &info
}

// GetRandomPeer fetches a random peer from the peers list
func (s *Server) GetRandomPeer() *peer.ID {
	s.peersLock.Lock()
	defer s.peersLock.Unlock()

	if len(s.peers) < 1 {
		return nil
	}

	randNum, _ := rand.Int(
		rand.Reader,
		big.NewInt(int64(len(s.peers))),
	)

	randomPeerIndx := int(randNum.Int64())

	counter := 0
	for peerID := range s.peers {
		if randomPeerIndx == counter {
			return &peerID
		}

		counter++
	}

	return nil
}

// FetchOrSetTemporaryDial loads the temporary status of a peer connection, and
// sets a new value [Thread safe]
func (s *Server) FetchOrSetTemporaryDial(peerID peer.ID, newValue bool) bool {
	_, loaded := s.temporaryDials.LoadOrStore(peerID, newValue)

	return loaded
}

// RemoveTemporaryDial removes a peer connection as temporary [Thread safe]
func (s *Server) RemoveTemporaryDial(peerID peer.ID) {
	s.temporaryDials.Delete(peerID)
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
		// spawn routine because PeerAdded is called from event handler and s.addToDialQueue emits event again
		go func() {
			info := s.host.Peerstore().PeerInfo(p)
			s.addToDialQueue(&info, common.PriorityRandomDial)
		}()
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
	)

	// Register a network event handler
	if err := s.Subscribe(context.Background(), discoveryService.HandleNetworkEvent); err != nil {
		return fmt.Errorf("unable to subscribe to network events, %w", err)
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

func (s *Server) TemporaryDialPeer(peerAddrInfo *peer.AddrInfo) {
	s.logger.Debug("creating new temporary dial to peer", "peer", peerAddrInfo.ID)
	s.addToDialQueue(peerAddrInfo, common.PriorityRandomDial)
}

// registerDiscoveryService registers the discovery protocol to be available
func (s *Server) registerDiscoveryService(discovery *discovery.DiscoveryService) {
	grpcStream := grpc.NewGrpcStream()
	proto.RegisterDiscoveryServer(grpcStream.GrpcServer(), discovery)
	grpcStream.Serve()

	s.RegisterProtocol(common.DiscProto, grpcStream)
}
