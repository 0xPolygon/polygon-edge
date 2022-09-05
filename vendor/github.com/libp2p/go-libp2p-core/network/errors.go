package network

import (
	"github.com/libp2p/go-libp2p/core/network"
)

// ErrNoRemoteAddrs is returned when there are no addresses associated with a peer during a dial.
// Deprecated: use github.com/libp2p/go-libp2p/core/network.ErrNoRemoteAddrs instead
var ErrNoRemoteAddrs = network.ErrNoRemoteAddrs

// ErrNoConn is returned when attempting to open a stream to a peer with the NoDial
// option and no usable connection is available.
// Deprecated: use github.com/libp2p/go-libp2p/core/network.ErrNoConn instead
var ErrNoConn = network.ErrNoConn

// ErrTransientConn is returned when attempting to open a stream to a peer with only a transient
// connection, without specifying the UseTransient option.
// Deprecated: use github.com/libp2p/go-libp2p/core/network.ErrTransientConn instead
var ErrTransientConn = network.ErrTransientConn

// ErrResourceLimitExceeded is returned when attempting to perform an operation that would
// exceed system resource limits.
// Deprecated: use github.com/libp2p/go-libp2p/core/network.ErrResourceLimitExceeded instead
var ErrResourceLimitExceeded = network.ErrResourceLimitExceeded

// ErrResourceScopeClosed is returned when attemptig to reserve resources in a closed resource
// scope.
// Deprecated: use github.com/libp2p/go-libp2p/core/network.ErrResourceScopeClosed instead
var ErrResourceScopeClosed = network.ErrResourceScopeClosed
