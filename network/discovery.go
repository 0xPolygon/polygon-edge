package network

import (
	"context"
	"math/rand"
	"sync"
	"time"

	"github.com/0xPolygon/minimal/network/grpc"
	"github.com/0xPolygon/minimal/network/proto"
	rawGrpc "google.golang.org/grpc"

	"github.com/libp2p/go-libp2p-core/peer"
	kb "github.com/libp2p/go-libp2p-kbucket"
	peerstore "github.com/libp2p/go-libp2p-peerstore"
)

func init() {
	rand.Seed(time.Now().Unix())
}

var discProto = "/disc/0.1"

const defaultBucketSize = 20

type discovery struct {
	proto.UnimplementedDiscoveryServer
	srv          *Server
	routingTable *kb.RoutingTable

	peers     []peer.ID
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
	d.peers = []peer.ID{}

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
		if evnt.Type != PeerEventConnected {
			return
		}
		peerID := evnt.PeerID

		// add peer to the routing table and to our local peer
		_, err := d.routingTable.TryAddPeer(peerID, false, false)
		if err != nil {
			d.srv.logger.Error("failed to add peer to routing table", "err", err)
			return
		}

		d.peersLock.Lock()
		d.peers = append(d.peers, peerID)
		d.peersLock.Unlock()
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

func (d *discovery) findPeersCall(peerID peer.ID) ([]*peer.AddrInfo, error) {
	conn, err := d.srv.NewProtoStream(discProto, peerID)
	if err != nil {
		return nil, err
	}
	clt := proto.NewDiscoveryClient(conn.(*rawGrpc.ClientConn))

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
			if err := d.call(target); err != nil {
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
