package network

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	rawGrpc "google.golang.org/grpc"

	"github.com/0xPolygon/polygon-edge/network/grpc"
	"github.com/0xPolygon/polygon-edge/network/proto"
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

	pending     sync.Map
	pendingSize int64

	srv *Server

	initialized uint32
}

func (i *identity) numPending() int64 {
	return atomic.LoadInt64(&i.pendingSize)
}

func (i *identity) isPending(id peer.ID) bool {
	val, ok := i.pending.Load(id)
	if !ok {
		return false
	}

	boolVal, ok := val.(bool)
	if !ok {
		return false
	}

	return boolVal
}

func (i *identity) delPending(id peer.ID) {
	if _, loaded := i.pending.LoadAndDelete(id); loaded {
		atomic.AddInt64(&i.pendingSize, -1)
	}
}

func (i *identity) setPending(id peer.ID) {
	if _, loaded := i.pending.LoadOrStore(id, true); !loaded {
		atomic.AddInt64(&i.pendingSize, 1)
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

			if i.srv.numOpenSlots() == 0 {
				i.srv.Disconnect(peerID, ErrNoAvailableSlots.Error())

				return
			}
			// pending of handshake
			i.setPending(peerID)

			go func() {
				defer func() {
					if i.isPending(peerID) {
						i.delPending(peerID)
						i.srv.emitEvent(peerID, PeerDialCompleted)
					}
				}()

				if err := i.handleConnected(peerID); err != nil {
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

func (i *identity) handleConnected(peerID peer.ID) error {
	// we initiated the connection, now we perform the handshake
	conn, err := i.srv.NewProtoStream(identityProtoV1, peerID)
	if err != nil {
		return err
	}

	rawGrpcConn, ok := conn.(*rawGrpc.ClientConn)
	if !ok {
		return errors.New("invalid type assert")
	}

	clt := proto.NewIdentityClient(rawGrpcConn)

	status := i.getStatus()
	resp, err := clt.Hello(context.Background(), status)

	if err != nil {
		return err
	}

	// validation
	if status.Chain != resp.Chain {
		return ErrInvalidChainID
	}

	i.srv.addPeer(peerID)

	return nil
}

func (i *identity) Hello(ctx context.Context, req *proto.Status) (*proto.Status, error) {
	return i.getStatus(), nil
}

func (i *identity) Bye(ctx context.Context, req *proto.ByeMsg) (*empty.Empty, error) {
	connContext, ok := ctx.(*grpc.Context)
	if !ok {
		return nil, errors.New("invalid type assert")
	}

	i.srv.logger.Debug("peer bye", "id", connContext.PeerID, "msg", req.Reason)

	return &empty.Empty{}, nil
}
