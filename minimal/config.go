package minimal

import (
	"net"

	"github.com/0xPolygon/minimal/chain"
	"github.com/0xPolygon/minimal/network"
)

const DefaultGRPCPort int = 9632
const DefaultJSONRPCPort int = 8545

// Config is used to parametrize the minimal client
type Config struct {
	Chain *chain.Chain

	JSONRPCAddr *net.TCPAddr
	GRPCAddr    *net.TCPAddr
	LibP2PAddr  *net.TCPAddr

	Network *network.Config
	DataDir string
	Seal    bool
}

// DefaultConfig returns the default config for JSON-RPC, GRPC (ports) and Networking
func DefaultConfig() *Config {
	return &Config{
		JSONRPCAddr: &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: DefaultJSONRPCPort},
		GRPCAddr:    &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: DefaultGRPCPort},
		Network:     network.DefaultConfig(),
	}
}
