package network

import (
	"context"
	"errors"
	"github.com/0xPolygon/polygon-edge/network/event"
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

const PeerID = "peerID"

var (
	ErrInvalidChainID   = errors.New("invalid chain ID")
	ErrNotReady         = errors.New("not ready")
	ErrNoAvailableSlots = errors.New("no available Slots")
)

type identity struct {
	proto.UnimplementedIdentityServer

	pending sync.Map
	srv     *Server

	initialized uint64
}

func (i *identity) isPending(id peer.ID) bool {
	_, ok := i.pending.Load(id)

	return ok
}

func (i *identity) removePendingStatus(peerID peer.ID) {
	if value, loaded := i.pending.LoadAndDelete(peerID); loaded {
		direction, ok := value.(network.Direction)
		if !ok {
			return
		}

		i.srv.UpdatePendingConnCount(-1, direction)
	}
}

func (i *identity) addPendingStatus(id peer.ID, direction network.Direction) {
	if _, loaded := i.pending.LoadOrStore(id, direction); !loaded {
		i.srv.UpdatePendingConnCount(1, direction)
	}
}

func (i *identity) setup() {
	// register the protobuf protocol
	grpc := grpc.NewGrpcStream()
	proto.RegisterIdentityServer(grpc.GrpcServer(), i)
	grpc.Serve()

	i.srv.RegisterProtocol(identityProtoV1, grpc)

	// register callback messages to notify from new peers
	// need to start our handshake protocol immediately but don't want to connect to any peer until initialized
	i.srv.host.Network().Notify(&network.NotifyBundle{
		ConnectedF: func(net network.Network, conn network.Conn) {
			peerID := conn.RemotePeer()
			i.srv.logger.Debug("Conn", "peer", peerID, "direction", conn.Stat().Direction)

			if atomic.LoadUint64(&i.initialized) == 0 {
				i.disconnectFromPeer(peerID, ErrNotReady.Error())

				return
			}

			if i.isPending(peerID) {
				// handshake has already started
				return
			}

			if !i.srv.HasFreeConnectionSlot(conn.Stat().Direction) {
				i.disconnectFromPeer(peerID, ErrNoAvailableSlots.Error())

				return
			}

			// Mark the peer as pending (pending handshake)
			i.addPendingStatus(peerID, conn.Stat().Direction)

			go func() {
				connectEvent := &event.PeerEvent{
					PeerID: peerID,
					Type:   event.PeerDialCompleted,
				}

				if err := i.handleConnected(peerID, conn.Stat().Direction); err != nil {
					// Close the connection to the peer
					i.disconnectFromPeer(peerID, err.Error())

					connectEvent.Type = event.PeerFailedToConnect
				}

				// Mark the peer as no longer pending
				i.removePendingStatus(connectEvent.PeerID)

				// Emit an adequate event
				i.srv.emitEvent(connectEvent.PeerID, connectEvent.Type)
			}()
		},
	})
}

func (i *identity) start() error {
	atomic.StoreUint64(&i.initialized, 1)

	return nil
}

func (i *identity) constructStatus(peerID peer.ID) *proto.Status {
	status := &proto.Status{
		Metadata: make(map[string]string, 1),
		Chain:    int64(i.srv.config.Chain.Params.ChainID),
	}
	if _, ok := i.srv.temporaryDials.Load(peerID); ok {
		status.TemporaryDial = true
	}

	return status
}

// disconnectFromPeer disconnects from the specified peer
func (i *identity) disconnectFromPeer(peerID peer.ID, reason string) {
	i.srv.Disconnect(peerID, reason)
}

func (i *identity) handleConnected(peerID peer.ID, direction network.Direction) error {
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

	status := i.constructStatus(peerID)

	status.Metadata[PeerID] = i.srv.host.ID().Pretty()

	resp, err := clt.Hello(context.Background(), status)
	if err != nil {
		return err
	}

	// validation
	if status.Chain != resp.Chain {
		return ErrInvalidChainID
	}

	if !resp.TemporaryDial && !status.TemporaryDial {
		i.srv.addPeer(peerID, direction)
	}

	return nil
}

func (i *identity) Hello(ctx context.Context, req *proto.Status) (*proto.Status, error) {
	peerID, err := peer.Decode(req.Metadata[PeerID])
	if err != nil {
		return nil, err
	}

	return i.constructStatus(peerID), nil
}

func (i *identity) Bye(ctx context.Context, req *proto.ByeMsg) (*empty.Empty, error) {
	connContext, ok := ctx.(*grpc.Context)
	if !ok {
		return nil, errors.New("invalid type assert")
	}

	i.srv.logger.Debug("peer bye", "id", connContext.PeerID, "msg", req.Reason)

	return &empty.Empty{}, nil
}
