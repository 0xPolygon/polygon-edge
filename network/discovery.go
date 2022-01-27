package network

import (
	"context"
	"sync/atomic"

	"crypto/rand"

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

type discovery struct {
	proto.UnimplementedDiscoveryServer
	srv          *Server
	routingTable *kb.RoutingTable // kademlia 'k-bucket' routing table that contains connected nodes info

	peers *referencePeers // list of the peers discovery queries about near peers

	closeCh chan struct{}

	bootnodes []*peer.AddrInfo

	bootnodeConnCount int32
}

func (d *discovery) setup(bootnodes []*peer.AddrInfo) error {
	d.peers = &referencePeers{}
	d.bootnodes = bootnodes

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

	d.routingTable.PeerAdded = func(p peer.ID) {
		info := d.srv.host.Peerstore().PeerInfo(p)
		d.srv.addToDialQueue(&info, PriorityRandomDial)
	}
	d.routingTable.PeerRemoved = func(p peer.ID) {
		d.srv.dialQueue.del(p)
	}

	grpc := grpc.NewGrpcStream()
	proto.RegisterDiscoveryServer(grpc.GrpcServer(), d)
	grpc.Serve()

	d.srv.Register(discProto, grpc)

	// send all the nodes we connect to the routing table
	err = d.srv.SubscribeFn(func(evnt *PeerEvent) {
		peerID := evnt.PeerID
		switch evnt.Type {
		case PeerConnected:
			// add peer to the routing table and to our local peer
			_, err := d.routingTable.TryAddPeer(peerID, false, false)
			if err != nil {
				d.srv.logger.Error("failed to add peer to routing table", "err", err)

				return
			}
			d.peers.add(peerID)
		case PeerDisconnected, PeerFailedToConnect:
			d.routingTable.RemovePeer(peerID)
			d.peers.delete(peerID)
		}
	})
	if err != nil {
		return err
	}

	go d.run()

	go d.setupTable()

	return nil
}

// isBootNode checks whether the given peer is bootnode or not
func (d *discovery) isBootNode(id peer.ID) bool {
	for _, v := range d.bootnodes {
		if v.ID == id {
			return true
		}
	}

	return false
}

func (d *discovery) addPeersToTable(nodes []string) error {
	for _, node := range nodes {
		info, err := StringToAddrInfo(node)
		if err != nil {
			d.srv.logger.Error("Failed to parse address", "err", err)

			continue
		}

		if err := d.addToTable(info); err != nil {
			d.srv.logger.Error("Failed to add new peer to routing table", "peer", info.ID, "err", err)
		}
	}

	return nil
}
func (d *discovery) addToTable(node *peer.AddrInfo) error {
	// before we include peers on the routing table -> dial queue
	// we have to add them to the peerstore so that they are
	// available to all the libp2p services
	d.srv.host.Peerstore().AddAddr(node.ID, node.Addrs[0], peerstore.AddressTTL)

	if _, err := d.routingTable.TryAddPeer(node.ID, false, false); err != nil {
		return err
	}

	return nil
}

func (d *discovery) setupTable() {
	for _, node := range d.bootnodes {
		if err := d.addToTable(node); err != nil {
			d.srv.logger.Error("Failed to add new peer to routing table", "peer", node.ID, "err", err)
		}
	}
}

func (d *discovery) attemptToFindPeers(peerID peer.ID) error {
	d.srv.logger.Debug("Querying a peer for near peers", "peer", peerID)
	nodes, err := d.findPeersCall(peerID)

	if err != nil {
		return err
	}

	d.srv.logger.Debug("Found new near peers", "peer", len(nodes))

	return d.addPeersToTable(nodes)
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

	clt := proto.NewDiscoveryClient(stream.(*rawGrpc.ClientConn))

	resp, err := clt.FindPeers(context.Background(), &proto.FindPeersReq{Count: defaultPeerReqCount})
	if err != nil {
		return nil, err
	}

	return resp.Nodes, nil
}

func (d *discovery) run() {
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
	return atomic.LoadInt32(&d.bootnodeConnCount)
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

	clt := proto.NewDiscoveryClient(stream.(*rawGrpc.ClientConn))

	resp, err := clt.FindPeers(context.Background(), &proto.FindPeersReq{Count: defaultPeerReqCount})
	if err != nil {
		d.srv.logger.Error("Find peers call failed", "peer", bootnode.ID, "err", err)

		return
	}

	if err := stream.(*rawGrpc.ClientConn).Close(); err != nil {
		d.srv.logger.Error("Error closing grpc stream", "peer", bootnode.ID, "err", err)

		return
	}

	if err = d.addPeersToTable(resp.Nodes); err != nil {
		d.srv.logger.Error("Failed to add peers to table", "err", err)
	}
}

func (d *discovery) FindPeers(
	ctx context.Context,
	req *proto.FindPeersReq,
) (*proto.FindPeersResp, error) {
	from := ctx.(*grpc.Context).PeerID

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
			info := d.srv.host.Peerstore().PeerInfo(id)
			filtered = append(filtered, AddrInfoToString(&info))
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
