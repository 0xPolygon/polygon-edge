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
	defaultPeerReqCount = 16

	peerDiscoveryInterval = 5 * time.Second

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
	GetBootnodeConnCount() int32

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

	// GetAvailableOutboundConnections fetches the number of available outbound connection slots [Thread safe]
	GetAvailableOutboundConnections() int64
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
		// Run cleanup
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

	if _, err := d.routingTable.TryAddPeer(node.ID, false, false); err != nil {
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
	nodes, err := d.findPeersCall(peerID)

	if err != nil {
		return err
	}

	d.logger.Debug("Found new near peers", "peer", len(nodes))
	d.addPeersToTable(nodes)

	return nil
}

func (d *DiscoveryService) getStream(peerID peer.ID) (interface{}, error) {
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

func (d *DiscoveryService) findPeersCall(peerID peer.ID) ([]string, error) {
	stream, err := d.getStream(peerID)
	if err != nil {
		return nil, err
	}

	rawGrpcConn, ok := stream.(*rawGrpc.ClientConn)
	if !ok {
		return nil, errors.New("invalid type assertion")
	}

	clt := proto.NewDiscoveryClient(rawGrpcConn)

	resp, err := clt.FindPeers(context.Background(), &proto.FindPeersReq{Count: defaultPeerReqCount})
	if err != nil {
		return nil, err
	}

	return resp.Nodes, nil
}

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
			go d.handleDiscovery()
		case <-bootnodeDiscoveryTicker.C:
			go d.bootnodeDiscovery()
		}
	}
}

func (d *DiscoveryService) handleDiscovery() {
	// take a random peer and find peers
	if d.baseServer.GetAvailableOutboundConnections() > 0 {
		if target := d.peers.getRandomPeer(); target != nil {
			if err := d.attemptToFindPeers(target.id); err != nil {
				d.logger.Error(
					"Failed to find new peers",
					"peer",
					target.id,
					"err",
					err,
				)
			}
		}
	}
}

// bootnodeDiscovery queries a random bootnode for new peers and adds them to the routing table
func (d *DiscoveryService) bootnodeDiscovery() {
	if d.baseServer.GetAvailableOutboundConnections() < 1 {
		// No need to attempt bootnode dialing, since no
		// open outbound slots are left
		return
	}

	var (
		candidateFound  bool           // a suitable query bootnode found
		isTemporaryDial bool           // dial status of the connection
		bootnode        *peer.AddrInfo // the reference bootnode
	)

	for !candidateFound {
		// get a random bootnode which is not connected
		bootnode = d.baseServer.GetRandomBootnode()
		if bootnode == nil {
			// No bootnodes available
			return
		}

		// If one or more bootnode is connected the dial status is temporary
		if d.baseServer.GetBootnodeConnCount() > 0 {
			isTemporaryDial = true
		}

		if isTemporaryDial {
			if loaded := d.baseServer.FetchAndSetTemporaryDial(bootnode.ID, true); loaded {
				return
			}
		}
	}

	defer func() {
		if isTemporaryDial {
			d.baseServer.RemoveTemporaryDial(bootnode.ID)
			d.baseServer.DisconnectFromPeer(bootnode.ID, "Thank you")
		}
	}()

	stream, err := d.baseServer.NewProtoStream(common.DiscProto, bootnode.ID)
	if err != nil {
		d.logger.Error("Failed to open new stream", "peer", bootnode.ID, "err", err)

		return
	}

	clientConnection, ok := stream.(*rawGrpc.ClientConn)
	if !ok {
		d.logger.Error("invalid type assertion for client connection")

		return
	}

	clt := proto.NewDiscoveryClient(clientConnection)

	resp, err := clt.FindPeers(context.Background(), &proto.FindPeersReq{
		Count: defaultPeerReqCount,
	})
	if err != nil {
		d.logger.Error("Find peers call failed", "peer", bootnode.ID, "err", err)

		return
	}

	if err := clientConnection.Close(); err != nil {
		d.logger.Error("Error closing grpc stream", "peer", bootnode.ID, "err", err)

		return
	}

	d.addPeersToTable(resp.Nodes)
}

func (d *DiscoveryService) FindPeers(
	ctx context.Context,
	req *proto.FindPeersReq,
) (*proto.FindPeersResp, error) {
	grpcContext, ok := ctx.(*grpc.Context)
	if !ok {
		return nil, errors.New("invalid type assertion")
	}

	from := grpcContext.PeerID

	if req.Count > 16 {
		// max limit
		req.Count = 16
	}

	if req.GetKey() == "" {
		// use peer id if none specified
		req.Key = from.String()
	}

	closer := d.routingTable.NearestPeers(kb.ConvertKey(req.GetKey()), int(req.Count))

	filtered := []string{}

	for _, id := range closer {
		// do not include himself
		if id != from {
			if info := d.baseServer.GetPeerInfo(id); len(info.Addrs) > 0 {
				filtered = append(filtered, common.AddrInfoToString(info))
			}
		}
	}

	resp := &proto.FindPeersResp{
		Nodes: filtered,
	}

	return resp, nil
}

func (d *DiscoveryService) Close() {
	close(d.closeCh)
}
