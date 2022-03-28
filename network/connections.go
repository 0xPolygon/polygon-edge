package network

import (
	"github.com/libp2p/go-libp2p-core/network"
	"sync/atomic"
)

// ConnectionInfo keeps track of current connection information
// for the networking server
type ConnectionInfo struct {
	// ACTIVE CONNECTIONS (VERIFIED) //
	inboundConnectionCount  int64
	outboundConnectionCount int64

	// PENDING CONNECTIONS (NOT YET VERIFIED) //
	// The reason for keeping pending connections is to, in a way, "reserve" open
	// slots for potential connections. These connections may or may not be valid in the end.
	// The additional complication of having the concept of pending (reserved) slots
	// is with calculating the number of actually available connection slots, in any direction.
	// This concept should be revisited in the future, as pending connections are not something
	// that is vital to the networking package or connection management as a whole
	pendingInboundConnectionCount  int64
	pendingOutboundConnectionCount int64

	// CONNECTION LIMITS //
	maxInboundConnectionCount  int64
	maxOutboundConnectionCount int64
}

// NewBlankConnectionInfo returns a cleared ConnectionInfo instance
func NewBlankConnectionInfo(
	maxInboundConnCount int64,
	maxOutboundConnCount int64,
) *ConnectionInfo {
	return &ConnectionInfo{
		inboundConnectionCount:         0,
		outboundConnectionCount:        0,
		pendingInboundConnectionCount:  0,
		pendingOutboundConnectionCount: 0,
		maxInboundConnectionCount:      maxInboundConnCount,
		maxOutboundConnectionCount:     maxOutboundConnCount,
	}
}

// GetInboundConnCount returns the number of active inbound connections [Thread safe]
func (ci *ConnectionInfo) GetInboundConnCount() int64 {
	return atomic.LoadInt64(&ci.inboundConnectionCount)
}

// GetOutboundConnCount returns the number of active outbound connections [Thread safe]
func (ci *ConnectionInfo) GetOutboundConnCount() int64 {
	return atomic.LoadInt64(&ci.outboundConnectionCount)
}

// GetPendingInboundConnCount returns the number of pending inbound connections [Thread safe]
func (ci *ConnectionInfo) GetPendingInboundConnCount() int64 {
	return atomic.LoadInt64(&ci.pendingInboundConnectionCount)
}

// GetPendingOutboundConnCount returns the number of pending outbound connections [Thread safe]
func (ci *ConnectionInfo) GetPendingOutboundConnCount() int64 {
	return atomic.LoadInt64(&ci.pendingOutboundConnectionCount)
}

// incInboundConnCount increases the number of inbound connections by delta [Thread safe]
func (ci *ConnectionInfo) incInboundConnCount(delta int64) {
	atomic.AddInt64(&ci.inboundConnectionCount, delta)
}

// incPendingInboundConnCount increases the number of pending inbound connections by delta [Thread safe]
func (ci *ConnectionInfo) incPendingInboundConnCount(delta int64) {
	atomic.AddInt64(&ci.pendingInboundConnectionCount, delta)
}

// incPendingOutboundConnCount increases the number of pending outbound connections by delta [Thread safe]
func (ci *ConnectionInfo) incPendingOutboundConnCount(delta int64) {
	atomic.AddInt64(&ci.pendingOutboundConnectionCount, delta)
}

// incOutboundConnCount increases the number of outbound connections by delta [Thread safe]
func (ci *ConnectionInfo) incOutboundConnCount(delta int64) {
	atomic.AddInt64(&ci.outboundConnectionCount, delta)
}

// HasFreeOutboundConn checks if there are any open outbound connection slots.
// It takes into account the number of current (active) outbound connections and
// the number of pending outbound connections [Thread safe]
func (ci *ConnectionInfo) HasFreeOutboundConn() bool {
	return ci.GetOutboundConnCount()+ci.GetPendingOutboundConnCount() < ci.maxOutboundConnCount()
}

// HasFreeInboundConn checks if there are any open inbound connection slots.
// It takes into account the number of current (active) inbound connections and
// the number of pending inbound connections [Thread safe]
func (ci *ConnectionInfo) HasFreeInboundConn() bool {
	return ci.GetInboundConnCount()+ci.GetPendingInboundConnCount() < ci.maxInboundConnCount()
}

// maxOutboundConnCount returns the maximum number of outbound connections.
// [Thread safe] since this value is unchanged during runtime
func (ci *ConnectionInfo) maxOutboundConnCount() int64 {
	return ci.maxOutboundConnectionCount
}

// maxInboundConnCount returns the minimum number of outbound connections
// [Thread safe] since this value is unchanged during runtime
func (ci *ConnectionInfo) maxInboundConnCount() int64 {
	return ci.maxInboundConnectionCount
}

// UpdateConnCountByDirection updates the connection count by delta
// in the specified direction [Thread safe]
func (ci *ConnectionInfo) UpdateConnCountByDirection(
	delta int64,
	direction network.Direction,
) {
	switch direction {
	case network.DirInbound:
		ci.incInboundConnCount(delta)
	case network.DirOutbound:
		ci.incOutboundConnCount(delta)
	}
}

// UpdatePendingConnCountByDirection updates the pending connection count by delta
// in the specified direction [Thread safe]
func (ci *ConnectionInfo) UpdatePendingConnCountByDirection(
	delta int64,
	direction network.Direction,
) {
	switch direction {
	case network.DirInbound:
		ci.incPendingInboundConnCount(delta)
	case network.DirOutbound:
		ci.incPendingOutboundConnCount(delta)
	}
}

// HasFreeConnectionSlot checks if there is a free connection slot in the
// specified direction [Thread safe]
func (ci *ConnectionInfo) HasFreeConnectionSlot(direction network.Direction) bool {
	switch direction {
	case network.DirInbound:
		return ci.HasFreeInboundConn()
	case network.DirOutbound:
		return ci.HasFreeOutboundConn()
	}

	return false
}
