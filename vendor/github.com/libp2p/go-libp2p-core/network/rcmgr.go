package network

import (
	"github.com/libp2p/go-libp2p/core/network"
)

// ResourceManager is the interface to the network resource management subsystem.
// The ResourceManager tracks and accounts for resource usage in the stack, from the internals
// to the application, and provides a mechanism to limit resource usage according to a user
// configurable policy.
//
// Resource Management through the ResourceManager is based on the concept of Resource
// Management Scopes, whereby resource usage is constrained by a DAG of scopes,
// The following diagram illustrates the structure of the resource constraint DAG:
// System
//
//	+------------> Transient.............+................+
//	|                                    .                .
//	+------------>  Service------------- . ----------+    .
//	|                                    .           |    .
//	+------------->  Protocol----------- . ----------+    .
//	|                                    .           |    .
//	+-------------->  Peer               \           |    .
//	                   +------------> Connection     |    .
//	                   |                             \    \
//	                   +--------------------------->  Stream
//
// The basic resources accounted by the ResourceManager include memory, streams, connections,
// and file  descriptors. These account for both space and time used by
// the stack, as each resource has a direct effect on the system
// availability and performance.
//
// The modus operandi of the resource manager is to restrict resource usage at the time of
// reservation. When a component of the stack needs to use a resource, it reserves it in the
// appropriate scope. The resource manager gates the reservation against the scope applicable
// limits; if the limit is exceeded, then an error (wrapping ErrResourceLimitExceeded) and it
// is up the component to act accordingly. At the lower levels of the stack, this will normally
// signal a failure of some sorts, like failing to opening a stream or a connection, which will
// propagate to the programmer. Some components may be able to handle resource reservation failure
// more gracefully; for instance a muxer trying to grow a buffer for a window change, will simply
// retain the existing window size and continue to operate normally albeit with some degraded
// throughput.
// All resources reserved in some scope are released when the scope is closed. For low level
// scopes, mainly Connection and Stream scopes, this happens when the connection or stream is
// closed.
//
// Service programmers will typically use the resource manager to reserve memory
// for their subsystem.
// This happens with two avenues: the programmer can attach a stream to a service, whereby
// resources reserved by the stream are automatically accounted in the service budget; or the
// programmer may directly interact with the service scope, by using ViewService through the
// resource manager interface.
//
// Application programmers can also directly reserve memory in some applicable scope. In order
// to facilitate control flow delimited resource accounting, all scopes defined in the system
// allow for the user to create spans. Spans are temporary scopes rooted at some
// other scope and release their resources when the programmer is done with them. Span
// scopes can form trees, with nested spans.
//
// Typical Usage:
//   - Low level components of the system (transports, muxers) all have access to the resource
//     manager and create connection and stream scopes through it. These scopes are accessible
//     to the user, albeit with a narrower interface, through Conn and Stream objects who have
//     a Scope method.
//   - Services typically center around streams, where the programmer can attach streams to a
//     particular service. They can also directly reserve memory for a service by accessing the
//     service scope using the ResourceManager interface.
//   - Applications that want to account for their network resource usage can reserve memory,
//     typically using a span, directly in the System or a Service scope; they can also
//     opt to use appropriate steam scopes for streams that they create or own.
//
// User Serviceable Parts: the user has the option to specify their own implementation of the
// interface. We provide a canonical implementation in the go-libp2p-resource-manager package.
// The user of that package can specify limits for the various scopes, which can be static
// or dynamic.
//
// WARNING The ResourceManager interface is considered experimental and subject to change
//
//	in subsequent releases.
//
// Deprecated: use github.com/libp2p/go-libp2p/core/network.ResourceManager instead
type ResourceManager = network.ResourceManager

// ResourceScopeViewer is a mixin interface providing view methods for accessing top level
// scopes.
// Deprecated: use github.com/libp2p/go-libp2p/core/network.ResourceScopeViewer instead
type ResourceScopeViewer = network.ResourceScopeViewer

const (
	// ReservationPriorityLow is a reservation priority that indicates a reservation if the scope
	// memory utilization is at 40% or less.
	// Deprecated: use github.com/libp2p/go-libp2p/core/network.ReservationPriorityLow instead
	ReservationPriorityLow = network.ReservationPriorityLow
	// ReservationPriorityMedium is a reservation priority that indicates a reservation if the scope
	// memory utilization is at 60% or less.
	// Deprecated: use github.com/libp2p/go-libp2p/core/network.ReservationPriorityMedium instead
	ReservationPriorityMedium uint8 = network.ReservationPriorityMedium
	// ReservationPriorityHigh is a reservation prioirity that indicates a reservation if the scope
	// memory utilization is at 80% or less.
	// Deprecated: use github.com/libp2p/go-libp2p/core/network.ReservationPriorityHigh instead
	ReservationPriorityHigh uint8 = network.ReservationPriorityHigh
	// ReservationPriorityAlways is a reservation priority that indicates a reservation if there is
	// enough memory, regardless of scope utilization.
	// Deprecated: use github.com/libp2p/go-libp2p/core/network.ReservationPriorityAlways instead
	ReservationPriorityAlways = network.ReservationPriorityAlways
)

// ResourceScope is the interface for all scopes.
// Deprecated: use github.com/libp2p/go-libp2p/core/network.ResourceScope instead
type ResourceScope = network.ResourceScope

// ResourceScopeSpan is a ResourceScope with a delimited span.
// Span scopes are control flow delimited and release all their associated resources
// when the programmer calls Done.
//
// Example:
//
//	s, err := someScope.BeginSpan()
//	if err != nil { ... }
//	defer s.Done()
//
//	if err := s.ReserveMemory(...); err != nil { ... }
//	// ... use memory
//
// Deprecated: use github.com/libp2p/go-libp2p/core/network.ResourceScopeSpan instead
type ResourceScopeSpan = network.ResourceScopeSpan

// ServiceScope is the interface for service resource scopes
// Deprecated: use github.com/libp2p/go-libp2p/core/network.ServiceScope instead
type ServiceScope = network.ServiceScope

// ProtocolScope is the interface for protocol resource scopes.
// Deprecated: use github.com/libp2p/go-libp2p/core/network.ProtocolScope instead
type ProtocolScope = network.ProtocolScope

// PeerScope is the interface for peer resource scopes.
// Deprecated: use github.com/libp2p/go-libp2p/core/network.PeerScope instead
type PeerScope = network.PeerScope

// ConnManagementScope is the low level interface for connection resource scopes.
// This interface is used by the low level components of the system who create and own
// the span of a connection scope.
// Deprecated: use github.com/libp2p/go-libp2p/core/network.ConnManagementScope instead
type ConnManagementScope = network.ConnManagementScope

// ConnScope is the user view of a connection scope
// Deprecated: use github.com/libp2p/go-libp2p/core/network.ConnScope instead
type ConnScope = network.ConnScope

// StreamManagementScope is the interface for stream resource scopes.
// This interface is used by the low level components of the system who create and own
// the span of a stream scope.
// Deprecated: use github.com/libp2p/go-libp2p/core/network.StreamManagementScope instead
type StreamManagementScope = network.StreamManagementScope

// StreamScope is the user view of a StreamScope.
// Deprecated: use github.com/libp2p/go-libp2p/core/network.StreamScope instead
type StreamScope = network.StreamScope

// ScopeStat is a struct containing resource accounting information.
// Deprecated: use github.com/libp2p/go-libp2p/core/network.ScopeStat instead
type ScopeStat = network.ScopeStat

// NullResourceManager is a stub for tests and initialization of default values
// Deprecated: use github.com/libp2p/go-libp2p/core/network.NullResourceManager instead
var NullResourceManager = network.NullResourceManager
