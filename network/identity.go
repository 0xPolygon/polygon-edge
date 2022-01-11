package network

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	rawGrpc "google.golang.org/grpc"

	"github.com/0xPolygon/polygon-sdk/network/grpc"
	"github.com/0xPolygon/polygon-sdk/network/proto"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	empty "google.golang.org/protobuf/types/known/emptypb"
)

var identityProtoV1 = "/id/0.1"

var (
	ErrInvalidChainID   = errors.New("invalid chain ID")
	ErrNotReady         = errors.New("not ready")
	ErrNoAvailableSlots = errors.New("no available Slots")
)

type identity struct {
	proto.UnimplementedIdentityServer

	pending sync.Map

	pendingInboundCount int64

	pendingOutboundCount int64

	srv *Server

	initialized uint32
}

func (i *identity) pendingInboundConns() int64 {

	return atomic.LoadInt64(&i.pendingInboundCount)
}

func (i *identity) pendingOutboundConns() int64 {

	return atomic.LoadInt64(&i.pendingOutboundCount)
}

func (i *identity) isPending(id peer.ID) bool {
	_, ok := i.pending.Load(id)
	return ok
}

func (i *identity) delPending(id peer.ID) {
	if value, loaded := i.pending.LoadAndDelete(id); loaded {
		if value.(network.Direction) == network.DirInbound {
			atomic.AddInt64(&i.pendingInboundCount, -1)
		} else if value.(network.Direction) == network.DirOutbound {
			atomic.AddInt64(&i.pendingOutboundCount, -1)
		}
	}
}

func (i *identity) setPending(id peer.ID, direction network.Direction) {
	if _, loaded := i.pending.LoadOrStore(id, direction); !loaded {
		if direction == network.DirInbound {
			atomic.AddInt64(&i.pendingInboundCount, 1)
		} else if direction == network.DirOutbound {
			atomic.AddInt64(&i.pendingOutboundCount, 1)
		}
	}
}

func (i *identity) setup() {
	// register the protobuf protocol
	grpc := grpc.NewGrpcStream()
	proto.RegisterIdentityServer(grpc.GrpcServer(), i)
	grpc.Serve()

	i.srv.Register(identityProtoV1, grpc)

	// register callback messages to notify from new peers
	// need to start our handshake protocol immediately but don't want to connect to any peer until initialized
	i.srv.host.Network().Notify(&network.NotifyBundle{
		ConnectedF: func(net network.Network, conn network.Conn) {
			peerID := conn.RemotePeer()
			i.srv.logger.Debug("Conn", "peer", peerID, "direction", conn.Stat().Direction)

			initialized := atomic.LoadUint32(&i.initialized)
			if initialized == 0 {
				i.srv.Disconnect(peerID, ErrNotReady.Error())

				return
			}

			// limit by MaxPeers on incoming / outgoing requests
			if i.isPending(peerID) {
				// handshake has already started
				return
			}

			if conn.Stat().Direction == network.DirOutbound && i.srv.numOpenSlots() == 0 {
				i.srv.Disconnect(peerID, ErrNoAvailableSlots.Error())
				return
			}

			if conn.Stat().Direction == network.DirInbound && i.srv.inboundConns() >= i.srv.maxInboundConns() {
				i.srv.Disconnect(peerID, ErrNoAvailableSlots.Error())

				return
			}
			// pending of handshake
			i.setPending(peerID, conn.Stat().Direction)

			go func() {
				defer func() {
					if i.isPending(peerID) {
						i.delPending(peerID)
						i.srv.emitEvent(peerID, PeerDialCompleted)
					}
				}()

				if err := i.handleConnected(peerID, conn.Stat().Direction); err != nil {
					i.srv.Disconnect(peerID, err.Error())
				}
			}()
		},
	})
}

func (i *identity) start() error {
	atomic.StoreUint32(&i.initialized, 1)

	return nil
}

func (i *identity) getStatus() *proto.Status {
	return &proto.Status{
		Chain: int64(i.srv.config.Chain.Params.ChainID),
	}
}

func (i *identity) handleConnected(peerID peer.ID, direction network.Direction) error {
	// we initiated the connection, now we perform the handshake
	conn, err := i.srv.NewProtoStream(identityProtoV1, peerID)
	if err != nil {
		return err
	}

	clt := proto.NewIdentityClient(conn.(*rawGrpc.ClientConn))

	status := i.getStatus()
	resp, err := clt.Hello(context.Background(), status)

	if err != nil {
		return err
	}

	// validation
	if status.Chain != resp.Chain {
		return ErrInvalidChainID
	}

	i.srv.addPeer(peerID, direction)

	return nil
}

func (i *identity) Hello(ctx context.Context, req *proto.Status) (*proto.Status, error) {
	return i.getStatus(), nil
}

func (i *identity) Bye(ctx context.Context, req *proto.ByeMsg) (*empty.Empty, error) {
	i.srv.logger.Debug("peer bye", "id", ctx.(*grpc.Context).PeerID, "msg", req.Reason)

	return &empty.Empty{}, nil
}
