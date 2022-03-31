package network

import (
	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/secrets"
	"github.com/multiformats/go-multiaddr"
	"net"
)

// Config details the params for the base networking server
type Config struct {
	NoDiscover       bool                   // flag indicating if the discovery mechanism should be turned on
	Addr             *net.TCPAddr           // the base address
	NatAddr          net.IP                 // the NAT address
	DNS              multiaddr.Multiaddr    // the DNS address
	DataDir          string                 // the base data directory for the client
	MaxPeers         int64                  // the maximum number of peer connections
	MaxInboundPeers  int64                  // the maximum number of inbound peer connections
	MaxOutboundPeers int64                  // the maximum number of outbound peer connections
	Chain            *chain.Chain           // the reference to the chain configuration
	SecretsManager   secrets.SecretsManager // the secrets manager used for key storage
	Metrics          *Metrics               // the metrics reporting reference
}

func DefaultConfig() *Config {
	return &Config{
		// The discovery service is turned on by default
		NoDiscover: false,
		// Addresses are bound to localhost by default
		Addr: &net.TCPAddr{
			IP:   net.ParseIP("127.0.0.1"),
			Port: DefaultLibp2pPort,
		},
		// The default ratio for outbound / max peer connections is 0.20
		MaxPeers: 40,
		// The default ratio for outbound / inbound connections is 0.25
		MaxInboundPeers:  32,
		MaxOutboundPeers: 8,
	}
}
