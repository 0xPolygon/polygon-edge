package network

import (
	"crypto/rand"
	"github.com/0xPolygon/polygon-edge/network/common"
	"github.com/0xPolygon/polygon-edge/network/proto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/peerstore"
	rawGrpc "google.golang.org/grpc"
	"math/big"
)

// IsBootnode checks if a peer is a bootnode [Thread safe]
func (s *Server) IsBootnode(peerID peer.ID) bool {
	return s.bootnodes.isBootnode(peerID)
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
	// Check if there is an active stream connection already
	if protoStream := s.getProtoStream(common.DiscProto, peerID); protoStream != nil {
		return proto.NewDiscoveryClient(protoStream), nil
	}

	// Create a new stream connection and return it
	protoStream, err := s.newProtoConnection(common.DiscProto, peerID)
	if err != nil {
		return nil, err
	}

	// Discovery protocol streams should be saved,
	// since they are referenced later on
	s.peersLock.Lock()
	connectionInfo := s.peers[peerID]
	connectionInfo.addProtocolStream(common.DiscProto, protoStream)
	s.peersLock.Unlock()

	return proto.NewDiscoveryClient(protoStream), nil
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
