package server

import (
	"net"

	"github.com/0xPolygon/polygon-sdk/chain"
	"github.com/0xPolygon/polygon-sdk/network"
	"github.com/0xPolygon/polygon-sdk/secrets"
	"github.com/0xPolygon/polygon-sdk/txpool"
)

const DefaultGRPCPort int = 9632
const DefaultJSONRPCPort int = 8545

// Config is used to parametrize the minimal client
type Config struct {
	Chain          *chain.Chain
	JSONRPCAddr    *net.TCPAddr
	GRPCAddr       *net.TCPAddr
	LibP2PAddr     *net.TCPAddr
	DataDir        string
	Seal           bool
	Telemetry      *Telemetry
	Network        *network.Config
	TxPool         *txpool.Config
	SecretsManager *secrets.SecretsManagerConfig
}

// DefaultConfig returns the default config for JSON-RPC, GRPC (ports) and Networking
func DefaultConfig() *Config {
	return &Config{
		JSONRPCAddr:    &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: DefaultJSONRPCPort},
		GRPCAddr:       &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: DefaultGRPCPort},
		Network:        network.DefaultConfig(),
		TxPool:         txpool.DefaultConfig(),
		Telemetry:      &Telemetry{PrometheusAddr: nil},
		SecretsManager: nil,
	}
}

// Telemetry holds the config details for metric services
type Telemetry struct {
	PrometheusAddr *net.TCPAddr
}
