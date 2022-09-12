package connmgr

import (
	"github.com/libp2p/go-libp2p/core/connmgr"
)

// ConnectionGater can be implemented by a type that supports active
// inbound or outbound connection gating.
//
// ConnectionGaters are active, whereas ConnManagers tend to be passive.
//
// A ConnectionGater will be consulted during different states in the lifecycle
// of a connection being established/upgraded. Specific functions will be called
// throughout the process, to allow you to intercept the connection at that stage.
//
//	InterceptPeerDial is called on an imminent outbound peer dial request, prior
//	to the addresses of that peer being available/resolved. Blocking connections
//	at this stage is typical for blacklisting scenarios.
//
//	InterceptAddrDial is called on an imminent outbound dial to a peer on a
//	particular address. Blocking connections at this stage is typical for
//	address filtering.
//
//	InterceptAccept is called as soon as a transport listener receives an
//	inbound connection request, before any upgrade takes place. Transports who
//	accept already secure and/or multiplexed connections (e.g. possibly QUIC)
//	MUST call this method regardless, for correctness/consistency.
//
//	InterceptSecured is called for both inbound and outbound connections,
//	after a security handshake has taken place and we've authenticated the peer.
//
//	InterceptUpgraded is called for inbound and outbound connections, after
//	libp2p has finished upgrading the connection entirely to a secure,
//	multiplexed channel.
//
// This interface can be used to implement *strict/active* connection management
// policies, such as hard limiting of connections once a maximum count has been
// reached, maintaining a peer blacklist, or limiting connections by transport
// quotas.
//
// EXPERIMENTAL: a DISCONNECT protocol/message will be supported in the future.
// This allows gaters and other components to communicate the intention behind
// a connection closure, to curtail potential reconnection attempts.
//
// For now, InterceptUpgraded can return a non-zero DisconnectReason when
// blocking a connection, but this interface is likely to change in the future
// as we solidify this feature. The reason why only this method can handle
// DisconnectReasons is that we require stream multiplexing capability to open a
// control protocol stream to transmit the message.
// Deprecated: use github.com/libp2p/go-libp2p/core/connmgr.ConnectionGater instead
type ConnectionGater = connmgr.ConnectionGater
