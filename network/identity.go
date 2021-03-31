package network

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/0xPolygon/minimal/network/grpc"
	"github.com/0xPolygon/minimal/network/proto"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/libp2p/go-libp2p-core/event"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
)

var identityProtoV1 = "/id/0.1"

type identity struct {
	proto.UnimplementedIdentityServer

	pending     sync.Map
	pendingSize int64

	srv *Server

	emitterPeer event.Emitter
	notifyCh    chan struct{}
}

func (i *identity) notify() {
	select {
	case i.notifyCh <- struct{}{}:
	default:
	}
}

func (i *identity) numPending() int64 {
	return atomic.LoadInt64(&i.pendingSize)
}

func (i *identity) delPending(id peer.ID) {
	i.pending.Delete(id)
	atomic.AddInt64(&i.pendingSize, -1)
}

func (i *identity) setPending(id peer.ID) {
	i.pending.Store(id, true)
	atomic.AddInt64(&i.pendingSize, 1)
}

func (i *identity) isPending(id peer.ID) bool {
	_, ok := i.pending.Load(id)
	return ok
}

func (i *identity) setup() {
	// register the protobuf protocol
	grpc := grpc.NewGrpcStream()
	proto.RegisterIdentityServer(grpc.GrpcServer(), i)

	i.srv.Register(identityProtoV1, grpc)

	emitterPeer, err := i.srv.host.EventBus().Emitter(new(PeerConnectedEvent))
	if err != nil {
		panic(err)
	}
	i.emitterPeer = emitterPeer

	// register callback messages to notify from new peers
	i.srv.host.Network().Notify(&network.NotifyBundle{
		ConnectedF: func(net network.Network, conn network.Conn) {
			// fmt.Printf("Conn: %s %s %d\n", conn.LocalPeer(), conn.RemotePeer(), conn.Stat().Direction)
			// pending of handshake
			peerID := conn.RemotePeer()
			i.setPending(peerID)

			go func() {
				time.Sleep(1 * time.Second)
				if err := i.handleConnected(peerID); err != nil {
					i.srv.Disconnect(peerID, err.Error())
				}
				// handshake is done
				i.delPending(peerID)
				i.notify()
			}()
		},
		DisconnectedF: func(net network.Network, conn network.Conn) {
			// remove from peers
			go func() {
				i.srv.delPeer(conn.RemotePeer())
				i.notify()
			}()
		},
	})
}

func (i *identity) getStatus() *proto.Status {
	return &proto.Status{
		Chain: int64(i.srv.config.Chain.Params.ChainID),
	}
}

func (i *identity) handleConnected(peerID peer.ID) error {
	// we initiated the connection, now we perform the handshake
	connxx := grpc.WrapClient(i.srv.StartStream(identityProtoV1, peerID))
	clt := proto.NewIdentityClient(connxx)

	status := i.getStatus()
	resp, err := clt.Hello(context.Background(), status)
	if err != nil {
		return err
	}

	// validation
	if status.Chain != resp.Chain {
		return fmt.Errorf("incorrect chain id")
	}

	i.srv.addPeer(peerID)
	i.emitterPeer.Emit(PeerConnectedEvent{Peer: peerID})
	return nil
}

func (i *identity) Hello(ctx context.Context, req *proto.Status) (*proto.Status, error) {
	return i.getStatus(), nil
}

func (i *identity) Bye(ctx context.Context, req *proto.ByeMsg) (*empty.Empty, error) {
	i.srv.logger.Debug("peer bye", "id", ctx.(*grpc.Context).PeerID, "msg", req.Reason)
	return &empty.Empty{}, nil
}
