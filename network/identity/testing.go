package identity

import (
	"context"
	"github.com/0xPolygon/polygon-edge/network/event"
	"github.com/0xPolygon/polygon-edge/network/proto"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"google.golang.org/grpc"
)

type mockNetworkingServer struct {
	// Mock identity client that simulates another peer
	mockClient *mockIdentityClient

	// Hooks that the test can set
	newIdentityClientFn      newIdentityClientDelegate
	disconnectFromPeerFn     disconnectFromPeerDelegate
	addPeerFn                addPeerDelegate
	updatePendingConnCountFn updatePendingConnCountDelegate
	emitEventFn              emitEventDelegate
	isTemporaryDialFn        isTemporaryDialDelegate
	hasFreeConnectionSlotFn  hasFreeConnectionSlotDelegate
}

func newMockNetworkingServer() *mockNetworkingServer {
	return &mockNetworkingServer{
		mockClient: &mockIdentityClient{},
	}
}

// Define the mock hooks
type newIdentityClientDelegate func(peer.ID) (proto.IdentityClient, error)
type disconnectFromPeerDelegate func(peer.ID, string)
type addPeerDelegate func(peer.ID, network.Direction)
type updatePendingConnCountDelegate func(int64, network.Direction)
type emitEventDelegate func(*event.PeerEvent)
type isTemporaryDialDelegate func(peerID peer.ID) bool
type hasFreeConnectionSlotDelegate func(direction network.Direction) bool

func (m *mockNetworkingServer) NewIdentityClient(peerID peer.ID) (proto.IdentityClient, error) {
	if m.newIdentityClientFn != nil {
		return m.newIdentityClientFn(peerID)
	}

	return m.mockClient, nil
}

func (m *mockNetworkingServer) HookNewIdentityClient(fn newIdentityClientDelegate) {
	m.newIdentityClientFn = fn
}

func (m *mockNetworkingServer) DisconnectFromPeer(peerID peer.ID, reason string) {
	if m.disconnectFromPeerFn != nil {
		m.disconnectFromPeerFn(peerID, reason)
	}
}

func (m *mockNetworkingServer) HookDisconnectFromPeer(fn disconnectFromPeerDelegate) {
	m.disconnectFromPeerFn = fn
}

func (m *mockNetworkingServer) AddPeer(id peer.ID, direction network.Direction) {
	if m.addPeerFn != nil {
		m.addPeerFn(id, direction)
	}
}

func (m *mockNetworkingServer) HookAddPeer(fn addPeerDelegate) {
	m.addPeerFn = fn
}

func (m *mockNetworkingServer) UpdatePendingConnCount(delta int64, direction network.Direction) {
	if m.updatePendingConnCountFn != nil {
		m.updatePendingConnCountFn(delta, direction)
	}
}

func (m *mockNetworkingServer) HookUpdatePendingConnCount(fn updatePendingConnCountDelegate) {
	m.updatePendingConnCountFn = fn
}

func (m *mockNetworkingServer) EmitEvent(event *event.PeerEvent) {
	if m.emitEventFn != nil {
		m.emitEventFn(event)
	}
}

func (m *mockNetworkingServer) HookEmitEvent(fn emitEventDelegate) {
	m.emitEventFn = fn
}

func (m *mockNetworkingServer) IsTemporaryDial(peerID peer.ID) bool {
	if m.isTemporaryDialFn != nil {
		return m.isTemporaryDialFn(peerID)
	}

	return false
}

func (m *mockNetworkingServer) HookIsTemporaryDial(fn isTemporaryDialDelegate) {
	m.isTemporaryDialFn = fn
}

func (m *mockNetworkingServer) HasFreeConnectionSlot(direction network.Direction) bool {
	if m.hasFreeConnectionSlotFn != nil {
		return m.hasFreeConnectionSlotFn(direction)
	}

	return true
}

func (m *mockNetworkingServer) HookHasFreeConnectionSlot(fn hasFreeConnectionSlotDelegate) {
	m.hasFreeConnectionSlotFn = fn
}

// mockIdentityClient mocks an identity client (other peer in the communication)
type mockIdentityClient struct {
	// Hooks that the test can set
	helloFn helloDelegate
}

type helloDelegate func(
	ctx context.Context,
	in *proto.Status,
	opts ...grpc.CallOption,
) (*proto.Status, error)

func (mic *mockIdentityClient) Hello(
	ctx context.Context,
	in *proto.Status,
	opts ...grpc.CallOption,
) (*proto.Status, error) {
	if mic.helloFn != nil {
		return mic.helloFn(ctx, in, opts...)
	}

	return nil, nil
}
