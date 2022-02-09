package server

import (
	"net"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/network"
	"github.com/0xPolygon/polygon-edge/secrets"
)

const DefaultGRPCPort int = 9632
const DefaultJSONRPCPort int = 8545
const DefaultBlockTime = 2 // in seconds

// Config is used to parametrize the minimal client
type Config struct {
	Chain *chain.Chain

	GRPCAddr       *net.TCPAddr
	LibP2PAddr     *net.TCPAddr
	Telemetry      *Telemetry
	Network        *network.Config
	DataDir        string
	Seal           bool
	PriceLimit     uint64
	MaxSlots       uint64
	SecretsManager *secrets.SecretsManagerConfig
	JSONRPC        *JSONRPC
	RestoreFile    *string
	BlockTime      uint64
}

// DefaultConfig returns the default config for JSON-RPC, GRPC (ports) and Networking
func DefaultConfig() *Config {
	return &Config{
		GRPCAddr:       &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: DefaultGRPCPort},
		Network:        network.DefaultConfig(),
		Telemetry:      &Telemetry{PrometheusAddr: nil},
		SecretsManager: nil,
		JSONRPC: &JSONRPC{
			JSONRPCAddr:              &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: DefaultJSONRPCPort},
			AccessControlAllowOrigin: nil,
		},
		BlockTime: DefaultBlockTime,
	}
}

// Telemetry holds the config details for metric services
type Telemetry struct {
	PrometheusAddr *net.TCPAddr
}

// JSONRPC holds the config details for the JSON-RPC server
type JSONRPC struct {
	JSONRPCAddr              *net.TCPAddr
	AccessControlAllowOrigin []string
}
