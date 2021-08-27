package network

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/0xPolygon/polygon-sdk/network/grpc"
	"github.com/0xPolygon/polygon-sdk/network/proto"
	rawGrpc "google.golang.org/grpc"

	"github.com/libp2p/go-libp2p-core/peer"
	peerstore "github.com/libp2p/go-libp2p-core/peerstore"
	kb "github.com/libp2p/go-libp2p-kbucket"
)

func init() {
	rand.Seed(time.Now().Unix())
}

var discProto = "/disc/0.1"

const defaultBucketSize = 20

// referencePeer is a representation of the peer the node is requesting for new peers
type referencePeer struct {
	id     peer.ID
	stream interface{}
}

type referencePeers []*referencePeer

func (ps *referencePeers) find(id peer.ID) *referencePeer {
	for _, p := range *ps {
		if p.id == id {
			return p
		}
	}
	return nil
}

func (ps *referencePeers) delete(id peer.ID) *referencePeer {
	idx := -1
	for i, p := range *ps {
		if p.id == id {
			idx = i
			break
		}
	}
	if idx != -1 {
		deletePeer := (*ps)[idx]
		(*ps) = append((*ps)[:idx], (*ps)[idx+1:]...)
		return deletePeer
	}
	return nil
}

type discovery struct {
	proto.UnimplementedDiscoveryServer
	srv          *Server
	routingTable *kb.RoutingTable

	peers     referencePeers
	peersLock sync.Mutex

	notifyCh chan struct{}
	closeCh  chan struct{}

	bootnodes []*peer.AddrInfo
}

func (d *discovery) setBootnodes(bootnodes []*peer.AddrInfo) {
	d.bootnodes = bootnodes
}

func (d *discovery) setup() error {
	d.notifyCh = make(chan struct{}, 5)
	d.peers = referencePeers{}

	keyID := kb.ConvertPeerID(d.srv.host.ID())

	routingTable, err := kb.NewRoutingTable(defaultBucketSize, keyID, time.Minute, d.srv.host.Peerstore(), 10*time.Second, nil)
	if err != nil {
		return err
	}
	d.routingTable = routingTable

	d.routingTable.PeerAdded = func(p peer.ID) {
		info := d.srv.host.Peerstore().PeerInfo(p)
		d.srv.dialQueue.add(&info, 10)
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
		case PeerEventConnected:
			// add peer to the routing table and to our local peer
			_, err := d.routingTable.TryAddPeer(peerID, false, false)
			if err != nil {
				d.srv.logger.Error("failed to add peer to routing table", "err", err)
				return
			}
			d.peersLock.Lock()
			d.peers = append(d.peers, &referencePeer{
				id:     peerID,
				stream: nil,
			})
			d.peersLock.Unlock()
		case PeerEventDisconnected:
			d.peersLock.Lock()
			d.peers.delete(peerID)
			d.peersLock.Unlock()
		}
	})
	if err != nil {
		return err
	}

	go d.run()

	return nil
}

func (d *discovery) call(peerID peer.ID) error {
	nodes, err := d.findPeersCall(peerID)
	if err != nil {
		return err
	}

	// before we include peers on the routing table -> dial queue
	// we have to add them to the peerstore so that they are
	// available to all the libp2p services
	for _, node := range nodes {
		d.srv.host.Peerstore().AddAddr(node.ID, node.Addrs[0], peerstore.AddressTTL)
		if _, err := d.routingTable.TryAddPeer(node.ID, false, false); err != nil {
			return err
		}
	}

	return nil
}

func (d *discovery) getStream(peerID peer.ID) (interface{}, error) {
	d.peersLock.Lock()
	defer d.peersLock.Unlock()

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

func (d *discovery) findPeersCall(peerID peer.ID) ([]*peer.AddrInfo, error) {
	stream, err := d.getStream(peerID)
	if err != nil {
		return nil, err
	}
	clt := proto.NewDiscoveryClient(stream.(*rawGrpc.ClientConn))

	resp, err := clt.FindPeers(context.Background(), &proto.FindPeersReq{Count: 16})
	if err != nil {
		return nil, err
	}

	var addrInfo []*peer.AddrInfo
	for _, node := range resp.Nodes {
		info, err := StringToAddrInfo(node)
		if err != nil {
			return nil, err
		}
		addrInfo = append(addrInfo, info)
	}

	return addrInfo, nil
}

func (d *discovery) run() {
	for {
		select {
		case <-time.After(5 * time.Second):
		case <-d.notifyCh:
		case <-d.closeCh:
			return
		}
		d.handleDiscovery()
	}
}

func (d *discovery) handleDiscovery() {
	if d.routingTable.Size() == 0 {
		// if there are no peers on the table try to include the bootnodes
		for _, node := range d.bootnodes {
			if _, err := d.routingTable.TryAddPeer(node.ID, false, false); err != nil {
				d.srv.logger.Error("failed to add bootnode", "err", err)
			}
		}
	} else {
		// take a random peer and find peers
		if len(d.peers) > 0 {
			target := d.peers[rand.Intn(len(d.peers))]
			if err := d.call(target.id); err != nil {
				d.srv.logger.Error("failed to dial bootnode", "err", err)
			}
		}
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
