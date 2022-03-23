package testing

import (
	"context"
	"github.com/0xPolygon/polygon-edge/network/event"
	"github.com/0xPolygon/polygon-edge/network/proto"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"google.golang.org/grpc"
)

type MockNetworkingServer struct {
	// Mock identity client that simulates another peer
	mockIdentityClient *MockIdentityClient

	// Hooks that the test can set
	newIdentityClientFn      newIdentityClientDelegate
	disconnectFromPeerFn     disconnectFromPeerDelegate
	addPeerFn                addPeerDelegate
	updatePendingConnCountFn updatePendingConnCountDelegate
	emitEventFn              emitEventDelegate
	isTemporaryDialFn        isTemporaryDialDelegate
	hasFreeConnectionSlotFn  hasFreeConnectionSlotDelegate
}

func NewMockNetworkingServer() *MockNetworkingServer {
	return &MockNetworkingServer{
		mockIdentityClient: &MockIdentityClient{},
	}
}

func (m *MockNetworkingServer) GetMockIdentityClient() *MockIdentityClient {
	return m.mockIdentityClient
}

// Define the mock hooks
type newIdentityClientDelegate func(peer.ID) (proto.IdentityClient, error)
type disconnectFromPeerDelegate func(peer.ID, string)
type addPeerDelegate func(peer.ID, network.Direction)
type updatePendingConnCountDelegate func(int64, network.Direction)
type emitEventDelegate func(*event.PeerEvent)
type isTemporaryDialDelegate func(peerID peer.ID) bool
type hasFreeConnectionSlotDelegate func(direction network.Direction) bool

func (m *MockNetworkingServer) NewIdentityClient(peerID peer.ID) (proto.IdentityClient, error) {
	if m.newIdentityClientFn != nil {
		return m.newIdentityClientFn(peerID)
	}

	return m.mockIdentityClient, nil
}

func (m *MockNetworkingServer) HookNewIdentityClient(fn newIdentityClientDelegate) {
	m.newIdentityClientFn = fn
}

func (m *MockNetworkingServer) DisconnectFromPeer(peerID peer.ID, reason string) {
	if m.disconnectFromPeerFn != nil {
		m.disconnectFromPeerFn(peerID, reason)
	}
}

func (m *MockNetworkingServer) HookDisconnectFromPeer(fn disconnectFromPeerDelegate) {
	m.disconnectFromPeerFn = fn
}

func (m *MockNetworkingServer) AddPeer(id peer.ID, direction network.Direction) {
	if m.addPeerFn != nil {
		m.addPeerFn(id, direction)
	}
}

func (m *MockNetworkingServer) HookAddPeer(fn addPeerDelegate) {
	m.addPeerFn = fn
}

func (m *MockNetworkingServer) UpdatePendingConnCount(delta int64, direction network.Direction) {
	if m.updatePendingConnCountFn != nil {
		m.updatePendingConnCountFn(delta, direction)
	}
}

func (m *MockNetworkingServer) HookUpdatePendingConnCount(fn updatePendingConnCountDelegate) {
	m.updatePendingConnCountFn = fn
}

func (m *MockNetworkingServer) EmitEvent(event *event.PeerEvent) {
	if m.emitEventFn != nil {
		m.emitEventFn(event)
	}
}

func (m *MockNetworkingServer) HookEmitEvent(fn emitEventDelegate) {
	m.emitEventFn = fn
}

func (m *MockNetworkingServer) IsTemporaryDial(peerID peer.ID) bool {
	if m.isTemporaryDialFn != nil {
		return m.isTemporaryDialFn(peerID)
	}

	return false
}

func (m *MockNetworkingServer) HookIsTemporaryDial(fn isTemporaryDialDelegate) {
	m.isTemporaryDialFn = fn
}

func (m *MockNetworkingServer) HasFreeConnectionSlot(direction network.Direction) bool {
	if m.hasFreeConnectionSlotFn != nil {
		return m.hasFreeConnectionSlotFn(direction)
	}

	return true
}

func (m *MockNetworkingServer) HookHasFreeConnectionSlot(fn hasFreeConnectionSlotDelegate) {
	m.hasFreeConnectionSlotFn = fn
}

// MockIdentityClient mocks an identity client (other peer in the communication)
type MockIdentityClient struct {
	// Hooks that the test can set
	helloFn helloDelegate
}

type helloDelegate func(
	ctx context.Context,
	in *proto.Status,
	opts ...grpc.CallOption,
) (*proto.Status, error)

func (mic *MockIdentityClient) HookHello(fn helloDelegate) {
	mic.helloFn = fn
}

func (mic *MockIdentityClient) Hello(
	ctx context.Context,
	in *proto.Status,
	opts ...grpc.CallOption,
) (*proto.Status, error) {
	if mic.helloFn != nil {
		return mic.helloFn(ctx, in, opts...)
	}

	return nil, nil
}
