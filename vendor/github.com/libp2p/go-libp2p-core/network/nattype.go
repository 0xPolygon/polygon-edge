package network

import "github.com/libp2p/go-libp2p/core/network"

// NATDeviceType indicates the type of the NAT device.
// Deprecated: use github.com/libp2p/go-libp2p/core/network.NATDeviceType instead
type NATDeviceType = network.NATDeviceType

const (
	// NATDeviceTypeUnknown indicates that the type of the NAT device is unknown.
	// Deprecated: use github.com/libp2p/go-libp2p/core/network.NATDeviceTypeUnknown instead
	NATDeviceTypeUnknown = network.NATDeviceTypeUnknown

	// NATDeviceTypeCone indicates that the NAT device is a Cone NAT.
	// A Cone NAT is a NAT where all outgoing connections from the same source IP address and port are mapped by the NAT device
	// to the same IP address and port irrespective of the destination address.
	// With regards to RFC 3489, this could be either a Full Cone NAT, a Restricted Cone NAT or a
	// Port Restricted Cone NAT. However, we do NOT differentiate between them here and simply classify all such NATs as a Cone NAT.
	// NAT traversal with hole punching is possible with a Cone NAT ONLY if the remote peer is ALSO behind a Cone NAT.
	// If the remote peer is behind a Symmetric NAT, hole punching will fail.
	// Deprecated: use github.com/libp2p/go-libp2p/core/network.NATDeviceTypeConn instead
	NATDeviceTypeCone = network.NATDeviceTypeCone

	// NATDeviceTypeSymmetric indicates that the NAT device is a Symmetric NAT.
	// A Symmetric NAT maps outgoing connections with different destination addresses to different IP addresses and ports,
	// even if they originate from the same source IP address and port.
	// NAT traversal with hole-punching is currently NOT possible in libp2p with Symmetric NATs irrespective of the remote peer's NAT type.
	// Deprecated: use github.com/libp2p/go-libp2p/core/network.NATDeviceTypeSymmetric instead
	NATDeviceTypeSymmetric = network.NATDeviceTypeSymmetric
)

// NATTransportProtocol is the transport protocol for which the NAT Device Type has been determined.
// Deprecated: use github.com/libp2p/go-libp2p/core/network.NATTransportProtocol instead
type NATTransportProtocol = network.NATTransportProtocol

const (
	// NATTransportUDP means that the NAT Device Type has been determined for the UDP Protocol.
	// Deprecated: use github.com/libp2p/go-libp2p/core/network.NATTransportUDP instead
	NATTransportUDP = network.NATTransportUDP
	// NATTransportTCP means that the NAT Device Type has been determined for the TCP Protocol.
	// Deprecated: use github.com/libp2p/go-libp2p/core/network.NATTransportTCP instead
	NATTransportTCP = network.NATTransportTCP
)
