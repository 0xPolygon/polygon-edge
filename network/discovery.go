package network

import (
	"context"
	"sync/atomic"

	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/0xPolygon/polygon-edge/network/grpc"
	"github.com/0xPolygon/polygon-edge/network/proto"
	"github.com/libp2p/go-libp2p-core/peer"
	peerstore "github.com/libp2p/go-libp2p-core/peerstore"
	kb "github.com/libp2p/go-libp2p-kbucket"
	rawGrpc "google.golang.org/grpc"
)

var discProto = "/disc/0.1"

const (
	defaultBucketSize = 20

	defaultPeerReqCount = 16

	peerDiscoveryInterval = 5 * time.Second

	bootnodeDiscoveryInterval = 60 * time.Second
)

type referencePeer struct {
	id     peer.ID
	stream interface{}
}

type referencePeers struct {
	mux   sync.RWMutex
	peers []*referencePeer
}

func (ps *referencePeers) find(id peer.ID) *referencePeer {
	ps.mux.RLock()
	defer ps.mux.RUnlock()

	for _, p := range ps.peers {
		if p.id == id {
			return p
		}
	}

	return nil
}

func (ps *referencePeers) getRandomPeer() *referencePeer {
	ps.mux.RLock()
	defer ps.mux.RUnlock()

	l := len(ps.peers)
	if l == 0 {
		return nil
	}

	randNum, _ := rand.Int(rand.Reader, big.NewInt(int64(l)))

	return ps.peers[randNum.Int64()]
}

func (ps *referencePeers) add(id peer.ID) {
	ps.mux.Lock()
	defer ps.mux.Unlock()

	ps.peers = append(ps.peers, &referencePeer{id: id, stream: nil})
}

func (ps *referencePeers) delete(id peer.ID) *referencePeer {
	ps.mux.Lock()
	defer ps.mux.Unlock()

	for idx, p := range ps.peers {
		if p.id == id {
			deletePeer := ps.peers[idx]
			ps.peers = append(ps.peers[:idx], ps.peers[idx+1:]...)

			return deletePeer
		}
	}

	return nil
}

type bootnodesWrapper struct {
	// bootnodeArr is the array that contains all the bootnode addresses
	bootnodeArr []*peer.AddrInfo

	// bootnodesMap is a map used for quick bootnode lookup
	bootnodesMap map[peer.ID]*peer.AddrInfo

	// bootnodeConnCount is an atomic value that keeps track
	// of the number of bootnode connections
	bootnodeConnCount int32
}

// isBootnode checks if the node ID belongs to a set bootnode
func (bw *bootnodesWrapper) isBootnode(nodeID peer.ID) bool {
	_, ok := bw.bootnodesMap[nodeID]

	return ok
}

// getBootnodeConnCount loads the bootnode connection count [Thread safe]
func (bw *bootnodesWrapper) getBootnodeConnCount() int32 {
	return atomic.LoadInt32(&bw.bootnodeConnCount)
}

// getRandomBootnode fetches a random bootnode from the bootnode set.
// If no bootnode is present, it returns nil
func (bw *bootnodesWrapper) getRandomBootnode() *peer.AddrInfo {
	if len(bw.bootnodeArr) < 1 {
		return nil
	}

	randNum, _ := rand.Int(rand.Reader, big.NewInt(int64(len(bw.bootnodeArr))))

	return bw.bootnodeArr[randNum.Int64()]
}

type discovery struct {
	proto.UnimplementedDiscoveryServer
	srv          *Server
	routingTable *kb.RoutingTable // kademlia 'k-bucket' routing table that contains connected nodes info

	peers *referencePeers // list of the peers discovery queries about near peers

	closeCh chan struct{}

	bootnodes *bootnodesWrapper
}

func (d *discovery) setup(bootnodes []*peer.AddrInfo) error {
	d.peers = &referencePeers{}
	d.setupBootnodesMap(bootnodes)

	// Set up a fresh routing table
	keyID := kb.ConvertPeerID(d.srv.host.ID())

	routingTable, err := kb.NewRoutingTable(
		defaultBucketSize,
		keyID,
		time.Minute,
		d.srv.host.Peerstore(),
		10*time.Second,
		nil,
	)
	if err != nil {
		return err
	}

	d.routingTable = routingTable

	// Set the PeerAdded event handler
	d.routingTable.PeerAdded = func(p peer.ID) {
		info := d.srv.host.Peerstore().PeerInfo(p)
		d.srv.addToDialQueue(&info, PriorityRandomDial)
	}

	// Set the PeerRemoved event handler
	d.routingTable.PeerRemoved = func(p peer.ID) {
		d.srv.dialQueue.del(p)
	}

	// Register the discovery service
	if serviceErr := d.registerDiscoveryService(); serviceErr != nil {
		return serviceErr
	}

	// Attempt connections to set bootnodes
	go d.connectToBootnodes()

	// Start the discovery mechanism
	go d.startDiscovery()

	return nil
}

// setupBootnodesMap initializes the discovery mechanism bootnode map
func (d *discovery) setupBootnodesMap(bootnodes []*peer.AddrInfo) {
	bootnodesMap := make(map[peer.ID]*peer.AddrInfo)

	for _, bootnodeInfo := range bootnodes {
		bootnodesMap[bootnodeInfo.ID] = bootnodeInfo
	}

	d.bootnodes = &bootnodesWrapper{
		bootnodeArr:       bootnodes,
		bootnodesMap:      bootnodesMap,
		bootnodeConnCount: 0,
	}
}

// registerDiscoveryService registers the gRPC discovery service.
// The discovery service enables a node to be queryable by other nodes for their peers
func (d *discovery) registerDiscoveryService() error {
	grpcStream := grpc.NewGrpcStream()
	proto.RegisterDiscoveryServer(grpcStream.GrpcServer(), d)
	grpcStream.Serve()

	d.srv.RegisterProtocol(discProto, grpcStream)

	// Send all the nodes we connect to the routing table
	return d.srv.SubscribeFn(func(evnt *PeerEvent) {
		peerID := evnt.PeerID
		switch evnt.Type {
		case PeerConnected:
			// Add peer to the routing table and to our local peer table
			_, err := d.routingTable.TryAddPeer(peerID, false, false)
			if err != nil {
				d.srv.logger.Error("failed to add peer to routing table", "err", err)

				return
			}

			d.peers.add(peerID)
		case PeerDisconnected, PeerFailedToConnect:
			// Run cleanup
			d.routingTable.RemovePeer(peerID)
			d.peers.delete(peerID)
		}
	})
}

// connectToBootnodes attempts to connect to the bootnodes
// and add them to the peer / routing table
func (d *discovery) connectToBootnodes() {
	for nodeID, nodeInfo := range d.bootnodes.bootnodesMap {
		if err := d.addToTable(nodeInfo); err != nil {
			d.srv.logger.Error(
				"Failed to add new peer to routing table",
				"peer",
				nodeID,
				"err",
				err,
			)
		}
	}
}

// addToTable adds the node to the peer store and the routing table
func (d *discovery) addToTable(node *peer.AddrInfo) error {
	// before we include peers on the routing table -> dial queue
	// we have to add them to the peer store so that they are
	// available to all the libp2p services
	d.srv.host.Peerstore().AddAddr(node.ID, node.Addrs[0], peerstore.AddressTTL)

	if _, err := d.routingTable.TryAddPeer(node.ID, false, false); err != nil {
		return err
	}

	return nil
}

// addPeersToTable adds the passed in peers to the peer store and the routing table
func (d *discovery) addPeersToTable(nodeAddrStrs []string) {
	for _, nodeAddrStr := range nodeAddrStrs {
		// Convert the string address info to
		// a working type
		nodeInfo, err := StringToAddrInfo(nodeAddrStr)
		if err != nil {
			d.srv.logger.Error(
				"Failed to parse address",
				"err",
				err,
			)

			continue
		}

		if err := d.addToTable(nodeInfo); err != nil {
			d.srv.logger.Error(
				"Failed to add new peer to routing table",
				"peer",
				nodeInfo.ID,
				"err",
				err,
			)
		}
	}
}

// isBootNode checks whether the given peer is bootnode or not
func (d *discovery) isBootnode(id peer.ID) bool {
	return d.bootnodes.isBootnode(id)
}

// attemptToFindPeers dials the specified peer and requests
// to see their peer list
func (d *discovery) attemptToFindPeers(peerID peer.ID) error {
	d.srv.logger.Debug("Querying a peer for near peers", "peer", peerID)
	nodes, err := d.findPeersCall(peerID)

	if err != nil {
		return err
	}

	d.srv.logger.Debug("Found new near peers", "peer", len(nodes))
	d.addPeersToTable(nodes)

	return nil
}

func (d *discovery) getStream(peerID peer.ID) (interface{}, error) {
	p := d.peers.find(peerID)
	if p == nil {
		return nil, fmt.Errorf("peer not found in list")
	}

	// return the existing stream if stream has been opened
	if p.stream != nil {
		return p.stream, nil
	}

	stream, err := d.srv.NewProtoStream(discProto, peerID)
	if err != nil {
		return nil, err
	}

	p.stream = stream

	return p.stream, nil
}

func (d *discovery) findPeersCall(peerID peer.ID) ([]string, error) {
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

func (d *discovery) startDiscovery() {
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

func (d *discovery) handleDiscovery() {
	// take a random peer and find peers
	if d.srv.availableOutboundConns() > 0 {
		if target := d.peers.getRandomPeer(); target != nil {
			if err := d.attemptToFindPeers(target.id); err != nil {
				d.srv.logger.Error("Failed to find new peers", "peer", target.id, "err", err)
			}
		}
	}
}

func (d *discovery) getBootNodeConnCount() int32 {
	return d.bootnodes.getBootnodeConnCount()
}

// bootnodeDiscovery queries a random bootnode for new peers and adds them to the routing table
func (d *discovery) bootnodeDiscovery() {
	if d.srv.availableOutboundConns() <= 0 {
		return
	}

	// get a random bootnode which is not connected
	bootnode := d.srv.getBootNode()
	if bootnode == nil {
		return
	}

	// isTemporaryDial maintains the dial status
	var isTemporaryDial bool

	// if one or more bootnode is connected the dial status is temporary
	if d.getBootNodeConnCount() >= 1 {
		isTemporaryDial = true
	}

	if isTemporaryDial {
		if _, loaded := d.srv.temporaryDials.LoadOrStore(bootnode.ID, true); loaded {
			return
		}
	}

	defer func() {
		if isTemporaryDial {
			d.srv.temporaryDials.Delete(bootnode.ID)
			d.srv.Disconnect(bootnode.ID, "Thank you")
		}
	}()

	if len(d.srv.host.Peerstore().Addrs(bootnode.ID)) == 0 {
		d.srv.host.Peerstore().AddAddr(bootnode.ID, bootnode.Addrs[0], peerstore.AddressTTL)
	}

	stream, err := d.srv.NewProtoStream(discProto, bootnode.ID)
	if err != nil {
		d.srv.logger.Error("Failed to open new stream", "peer", bootnode.ID, "err", err)

		return
	}

	clientConnection, ok := stream.(*rawGrpc.ClientConn)
	if !ok {
		d.srv.logger.Error("invalid type assertion for client connection")

		return
	}

	clt := proto.NewDiscoveryClient(clientConnection)

	resp, err := clt.FindPeers(context.Background(), &proto.FindPeersReq{Count: defaultPeerReqCount})
	if err != nil {
		d.srv.logger.Error("Find peers call failed", "peer", bootnode.ID, "err", err)

		return
	}

	if err := clientConnection.Close(); err != nil {
		d.srv.logger.Error("Error closing grpc stream", "peer", bootnode.ID, "err", err)

		return
	}

	d.addPeersToTable(resp.Nodes)
}

func (d *discovery) FindPeers(
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
			if info := d.srv.host.Peerstore().PeerInfo(id); len(info.Addrs) > 0 {
				filtered = append(filtered, AddrInfoToString(&info))
			}
		}
	}

	resp := &proto.FindPeersResp{
		Nodes: filtered,
	}

	return resp, nil
}

func (d *discovery) Close() {
	close(d.closeCh)
}
