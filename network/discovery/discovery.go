package discovery

import (
	"context"
	"errors"
	"fmt"
	"github.com/0xPolygon/polygon-edge/network/common"
	"github.com/0xPolygon/polygon-edge/network/event"
	"github.com/hashicorp/go-hclog"
	"time"

	"github.com/0xPolygon/polygon-edge/network/grpc"
	"github.com/0xPolygon/polygon-edge/network/proto"
	"github.com/libp2p/go-libp2p-core/peer"
	kb "github.com/libp2p/go-libp2p-kbucket"
	rawGrpc "google.golang.org/grpc"
)

const (
	// maxDiscoveryPeerReqCount is the max peer count that
	// can be requested from other peers
	maxDiscoveryPeerReqCount = 16

	// peerDiscoveryInterval is the interval at which other
	// peers are queried for their peer sets
	peerDiscoveryInterval = 5 * time.Second

	// bootnodeDiscoveryInterval is the interval at which
	// random bootnodes are dialed for their peer sets
	bootnodeDiscoveryInterval = 60 * time.Second
)

// networkingServer defines the base communication interface between
// any networking server implementation and the DiscoveryService
type networkingServer interface {
	// BOOTNODE QUERIES //

	// IsBootnode checks if the peer is a registered bootnode
	IsBootnode(peerID peer.ID) bool

	// GetRandomBootnode fetches a random bootnode, if any
	GetRandomBootnode() *peer.AddrInfo

	// GetBootnodeConnCount fetches the number of bootnode connections [Thread safe]
	GetBootnodeConnCount() int64

	// PROTOCOL MANIPULATION //

	// NewProtoStream opens a new protocol stream towards the referenced peer
	NewProtoStream(protocol string, peerID peer.ID) (interface{}, error)

	// PEER MANIPULATION //

	// DisconnectFromPeer attempts to disconnect from the specified peer
	DisconnectFromPeer(peerID peer.ID, reason string)

	// AddToPeerStore adds a peer to the networking server's peer store
	AddToPeerStore(peerInfo *peer.AddrInfo)

	// GetPeerInfo fetches the peer information from the server's peer store
	GetPeerInfo(peerID peer.ID) *peer.AddrInfo

	// TEMPORARY DIALING //

	// FetchAndSetTemporaryDial checks if the peer connection is a temporary dial,
	// and sets a new value accordingly
	FetchAndSetTemporaryDial(peerID peer.ID, newValue bool) bool

	// RemoveTemporaryDial removes a peer from the temporary dial map
	RemoveTemporaryDial(peerID peer.ID)

	// CONNECTION INFORMATION //

	// HasFreeOutboundConnections checks if there are available outbound connection slots [Thread safe]
	HasFreeOutboundConnections() bool
}

// DiscoveryService is a service that finds other peers in the network
// and connects them to the current running node
type DiscoveryService struct {
	proto.UnimplementedDiscoveryServer

	baseServer   networkingServer // The interface towards the base networking server
	logger       hclog.Logger     // The DiscoveryService logger
	routingTable *kb.RoutingTable // Kademlia 'k-bucket' routing table that contains connected nodes info
	peers        *referencePeers  // List of the peers DiscoveryService queries about near peers

	closeCh chan struct{} // Channel used for stopping the DiscoveryService
}

// NewDiscoveryService creates a new instance of the discovery service
func NewDiscoveryService(
	server networkingServer,
	routingTable *kb.RoutingTable,
	logger hclog.Logger,
	closeCh chan struct{},
) *DiscoveryService {
	return &DiscoveryService{
		logger: logger.Named("discovery"),
		peers: &referencePeers{
			peersMap: make(map[peer.ID]*referencePeer),
		},
		baseServer:   server,
		routingTable: routingTable,
		closeCh:      closeCh,
	}
}

// Start starts the discovery service
func (d *DiscoveryService) Start() {
	go d.startDiscovery()
}

// Close stops the discovery service
func (d *DiscoveryService) Close() {
	close(d.closeCh)
}

// RoutingTableSize returns the size of the routing table
func (d *DiscoveryService) RoutingTableSize() int {
	if d.routingTable == nil {
		return 0
	}

	return d.routingTable.Size()
}

// RoutingTablePeers fetches the peers from the routing table
func (d *DiscoveryService) RoutingTablePeers() []peer.ID {
	if d.routingTable == nil {
		return []peer.ID{}
	}

	return d.routingTable.ListPeers()
}

// HandleNetworkEvent handles base network events for the DiscoveryService
func (d *DiscoveryService) HandleNetworkEvent(peerEvent *event.PeerEvent) {
	peerID := peerEvent.PeerID

	switch peerEvent.Type {
	case event.PeerConnected:
		// Add peer to the routing table and to our local peer table
		_, err := d.routingTable.TryAddPeer(peerID, false, false)
		if err != nil {
			d.logger.Error("failed to add peer to routing table", "err", err)

			return
		}

		d.peers.addPeer(peerID)
	case event.PeerDisconnected, event.PeerFailedToConnect:
		// Run cleanup for the local routing / reference peers table
		d.routingTable.RemovePeer(peerID)
		d.peers.deletePeer(peerID)
	}
}

// ConnectToBootnodes attempts to connect to the bootnodes
// and add them to the peer / routing table
func (d *DiscoveryService) ConnectToBootnodes(bootnodes []*peer.AddrInfo) {
	for _, nodeInfo := range bootnodes {
		if err := d.addToTable(nodeInfo); err != nil {
			d.logger.Error(
				"Failed to add new peer to routing table",
				"peer",
				nodeInfo.ID,
				"err",
				err,
			)
		}
	}
}

// addToTable adds the node to the peer store and the routing table
func (d *DiscoveryService) addToTable(node *peer.AddrInfo) error {
	// before we include peers on the routing table -> dial queue
	// we have to add them to the peer store so that they are
	// available to all the libp2p services
	d.baseServer.AddToPeerStore(node)

	if _, err := d.routingTable.TryAddPeer(
		node.ID,
		false,
		false,
	); err != nil {
		return err
	}

	return nil
}

// addPeersToTable adds the passed in peers to the peer store and the routing table
func (d *DiscoveryService) addPeersToTable(nodeAddrStrs []string) {
	for _, nodeAddrStr := range nodeAddrStrs {
		// Convert the string address info to a working type
		nodeInfo, err := common.StringToAddrInfo(nodeAddrStr)
		if err != nil {
			d.logger.Error(
				"Failed to parse address",
				"err",
				err,
			)

			continue
		}

		if err := d.addToTable(nodeInfo); err != nil {
			d.logger.Error(
				"Failed to add new peer to routing table",
				"peer",
				nodeInfo.ID,
				"err",
				err,
			)
		}
	}
}

// attemptToFindPeers dials the specified peer and requests
// to see their peer list
func (d *DiscoveryService) attemptToFindPeers(peerID peer.ID) error {
	d.logger.Debug("Querying a peer for near peers", "peer", peerID)
	nodes, err := d.findPeersCall(peerID, false)

	if err != nil {
		return err
	}

	d.logger.Debug("Found new near peers", "peer", len(nodes))
	d.addPeersToTable(nodes)

	return nil
}

// getPeerStream fetches a stream to a peer (if it exists), or
// it opens a new one
func (d *DiscoveryService) getPeerStream(peerID peer.ID) (interface{}, error) {
	p := d.peers.isReferencePeer(peerID)
	if p == nil {
		return nil, fmt.Errorf("peer not found in list")
	}

	// return the existing stream if stream has been opened
	if p.stream != nil {
		return p.stream, nil
	}

	stream, err := d.baseServer.NewProtoStream(common.DiscProto, peerID)
	if err != nil {
		return nil, err
	}

	p.stream = stream

	return p.stream, nil
}

// findPeersCall queries the set peer for their peer set
func (d *DiscoveryService) findPeersCall(
	peerID peer.ID,
	shouldCloseConn bool,
) ([]string, error) {
	stream, err := d.getPeerStream(peerID)
	if err != nil {
		return nil, err
	}

	rawGrpcConn, ok := stream.(*rawGrpc.ClientConn)
	if !ok {
		return nil, errors.New("invalid type assertion")
	}

	clt := proto.NewDiscoveryClient(rawGrpcConn)

	resp, err := clt.FindPeers(context.Background(), &proto.FindPeersReq{Count: maxDiscoveryPeerReqCount})
	if err != nil {
		return nil, err
	}

	// Check if the connection should be closed after getting the data
	if shouldCloseConn {
		if closeErr := rawGrpcConn.Close(); closeErr != nil {
			return nil, closeErr
		}
	}

	return resp.Nodes, nil
}

// startDiscovery starts the DiscoveryService loop,
// in which random peers are dialed for their peer sets,
// and random bootnodes are dialed for their peer sets
func (d *DiscoveryService) startDiscovery() {
	peerDiscoveryTicker := time.NewTicker(peerDiscoveryInterval)
	bootnodeDiscoveryTicker := time.NewTicker(bootnodeDiscoveryInterval)

	defer func() {
		peerDiscoveryTicker.Stop()
		bootnodeDiscoveryTicker.Stop()
	}()

	for {
		select {
		case <-d.closeCh:
			return
		case <-peerDiscoveryTicker.C:
			go d.regularPeerDiscovery()
		case <-bootnodeDiscoveryTicker.C:
			go d.bootnodePeerDiscovery()
		}
	}
}

// regularPeerDiscovery grabs a random peer from the list of
// connected peers, and attempts to find / connect to their peer set
func (d *DiscoveryService) regularPeerDiscovery() {
	if !d.baseServer.HasFreeOutboundConnections() {
		// No need to do peer discovery if no open connection slots
		// are available
		return
	}

	// Grab a random peer from the current peer set to use as a reference
	refPeer := d.peers.getRandomPeer()
	if refPeer == nil {
		// The node cannot find a random peer to query
		// from the current peer set
		return
	}

	// Try to discover the peers connected to the reference peer
	if err := d.attemptToFindPeers(refPeer.id); err != nil {
		d.logger.Error(
			"Failed to find new peers",
			"peer",
			refPeer.id,
			"err",
			err,
		)
	}
}

// bootnodeDiscovery queries a random (unconnected) bootnode for new peers
// and adds them to the routing table
func (d *DiscoveryService) bootnodePeerDiscovery() {
	if !d.baseServer.HasFreeOutboundConnections() {
		// No need to attempt bootnode dialing, since no
		// open outbound slots are left
		return
	}

	var (
		isTemporaryDial bool           // dial status of the connection
		bootnode        *peer.AddrInfo // the reference bootnode
	)

	// Try to find a suitable bootnode to use as a reference peer
	for bootnode == nil {
		// Get a random unconnected bootnode from the bootnode set
		bootnode = d.baseServer.GetRandomBootnode()
		if bootnode == nil {
			// No bootnodes available
			return
		}

		// If one or more bootnode is connected the dial status is temporary
		if d.baseServer.GetBootnodeConnCount() > 0 {
			// Check if the peer is already a temporary dial
			if alreadyTempDial := d.baseServer.FetchAndSetTemporaryDial(
				bootnode.ID,
				true,
			); alreadyTempDial {
				continue
			}

			// Mark the subsequent connection as temporary
			isTemporaryDial = true
		}
	}

	defer func() {
		if isTemporaryDial {
			// Since temporary dials are short-lived, the connection
			// needs to be turned off the moment it's not needed anymore
			d.baseServer.RemoveTemporaryDial(bootnode.ID)
			d.baseServer.DisconnectFromPeer(bootnode.ID, "Thank you")
		}
	}()

	// Find peers from the referenced bootnode
	foundNodes, err := d.findPeersCall(bootnode.ID, true)
	if err != nil {
		d.logger.Error("Unable to execute bootnode peer discovery, %w", err)

		return
	}

	// Save the peers for subsequent dialing
	d.addPeersToTable(foundNodes)
}

// FindPeers implements the proto service for finding the target's peers
func (d *DiscoveryService) FindPeers(
	ctx context.Context,
	req *proto.FindPeersReq,
) (*proto.FindPeersResp, error) {
	// Extract the requesting peer ID from the gRPC context
	grpcContext, ok := ctx.(*grpc.Context)
	if !ok {
		return nil, errors.New("invalid type assertion")
	}

	from := grpcContext.PeerID

	// Sanity check for result set size
	if req.Count > maxDiscoveryPeerReqCount {
		req.Count = maxDiscoveryPeerReqCount
	}

	// The request Key is used for finding the closest peers
	// by utilizing Kademlia's distance calculation.
	// This way, the peer that's being queried for its peers delivers
	// only the closest ones to the requested key (peer)
	if req.GetKey() == "" {
		req.Key = from.String()
	}

	nearestPeers := d.routingTable.NearestPeers(
		kb.ConvertKey(req.GetKey()),
		int(req.Count),
	)

	// The peer that's initializing this request
	// doesn't need to be a part of the resulting set
	filteredPeers := make([]string, 0)

	for _, id := range nearestPeers {
		if id == from {
			// Skip the peer that's initializing the request
			continue
		}

		if info := d.baseServer.GetPeerInfo(id); len(info.Addrs) > 0 {
			filteredPeers = append(filteredPeers, common.AddrInfoToString(info))
		}
	}

	return &proto.FindPeersResp{
		Nodes: filteredPeers,
	}, nil
}
