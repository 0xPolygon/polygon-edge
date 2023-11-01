package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/0xPolygon/polygon-edge/network"
	"github.com/hashicorp/hcl"
	"gopkg.in/yaml.v3"
)

// Config defines the server configuration params
type Config struct {
	GenesisPath              string     `json:"chain_config" yaml:"chain_config"`
	SecretsConfigPath        string     `json:"secrets_config" yaml:"secrets_config"`
	DataDir                  string     `json:"data_dir" yaml:"data_dir"`
	BlockGasTarget           string     `json:"block_gas_target" yaml:"block_gas_target"`
	GRPCAddr                 string     `json:"grpc_addr" yaml:"grpc_addr"`
	JSONRPCAddr              string     `json:"jsonrpc_addr" yaml:"jsonrpc_addr"`
	Telemetry                *Telemetry `json:"telemetry" yaml:"telemetry"`
	Network                  *Network   `json:"network" yaml:"network"`
	ShouldSeal               bool       `json:"seal" yaml:"seal"`
	TxPool                   *TxPool    `json:"tx_pool" yaml:"tx_pool"`
	LogLevel                 string     `json:"log_level" yaml:"log_level"`
	RestoreFile              string     `json:"restore_file" yaml:"restore_file"`
	Headers                  *Headers   `json:"headers" yaml:"headers"`
	LogFilePath              string     `json:"log_to" yaml:"log_to"`
	JSONRPCBatchRequestLimit uint64     `json:"json_rpc_batch_request_limit" yaml:"json_rpc_batch_request_limit"`
	JSONRPCBlockRangeLimit   uint64     `json:"json_rpc_block_range_limit" yaml:"json_rpc_block_range_limit"`
	JSONLogFormat            bool       `json:"json_log_format" yaml:"json_log_format"`
	CorsAllowedOrigins       []string   `json:"cors_allowed_origins" yaml:"cors_allowed_origins"`

	Relayer               bool   `json:"relayer" yaml:"relayer"`
	NumBlockConfirmations uint64 `json:"num_block_confirmations" yaml:"num_block_confirmations"`

	ConcurrentRequestsDebug uint64 `json:"concurrent_requests_debug" yaml:"concurrent_requests_debug"`
	WebSocketReadLimit      uint64 `json:"web_socket_read_limit" yaml:"web_socket_read_limit"`

	MetricsInterval time.Duration `json:"metrics_interval" yaml:"metrics_interval"`
}

// Telemetry holds the config details for metric services.
type Telemetry struct {
	PrometheusAddr string `json:"prometheus_addr" yaml:"prometheus_addr"`
}

// Network defines the network configuration params
type Network struct {
	NoDiscover       bool   `json:"no_discover" yaml:"no_discover"`
	Libp2pAddr       string `json:"libp2p_addr" yaml:"libp2p_addr"`
	NatAddr          string `json:"nat_addr" yaml:"nat_addr"`
	DNSAddr          string `json:"dns_addr" yaml:"dns_addr"`
	MaxPeers         int64  `json:"max_peers,omitempty" yaml:"max_peers,omitempty"`
	MaxOutboundPeers int64  `json:"max_outbound_peers,omitempty" yaml:"max_outbound_peers,omitempty"`
	MaxInboundPeers  int64  `json:"max_inbound_peers,omitempty" yaml:"max_inbound_peers,omitempty"`
}

// TxPool defines the TxPool configuration params
type TxPool struct {
	PriceLimit         uint64 `json:"price_limit" yaml:"price_limit"`
	MaxSlots           uint64 `json:"max_slots" yaml:"max_slots"`
	MaxAccountEnqueued uint64 `json:"max_account_enqueued" yaml:"max_account_enqueued"`
}

// Headers defines the HTTP response headers required to enable CORS.
type Headers struct {
	AccessControlAllowOrigins []string `json:"access_control_allow_origins" yaml:"access_control_allow_origins"`
}

const (
	// BlockTimeMultiplierForTimeout Multiplier to get IBFT timeout from block time
	// timeout is calculated when IBFT timeout is not specified
	BlockTimeMultiplierForTimeout uint64 = 5

	// DefaultJSONRPCBatchRequestLimit maximum length allowed for json_rpc batch requests
	DefaultJSONRPCBatchRequestLimit uint64 = 20

	// DefaultJSONRPCBlockRangeLimit maximum block range allowed for json_rpc
	// requests with fromBlock/toBlock values (e.g. eth_getLogs)
	DefaultJSONRPCBlockRangeLimit uint64 = 1000

	// DefaultNumBlockConfirmations minimal number of child blocks required for the parent block to be considered final
	// on ethereum epoch lasts for 32 blocks. more details: https://www.alchemy.com/overviews/ethereum-commitment-levels
	DefaultNumBlockConfirmations uint64 = 64

	// DefaultConcurrentRequestsDebug specifies max number of allowed concurrent requests for debug endpoints
	DefaultConcurrentRequestsDebug uint64 = 32

	// DefaultWebSocketReadLimit specifies max size in bytes for a message read from the peer by Gorrila websocket lib.
	// If a message exceeds the limit,
	// the connection sends a close message to the peer and returns ErrReadLimit to the application.
	DefaultWebSocketReadLimit uint64 = 8192

	// DefaultMetricsInterval specifies the time interval after which Prometheus metrics will be generated.
	// A value of 0 means the metrics are disabled.
	DefaultMetricsInterval time.Duration = time.Second * 8
)

// DefaultConfig returns the default server configuration
func DefaultConfig() *Config {
	defaultNetworkConfig := network.DefaultConfig()

	return &Config{
		GenesisPath:    "./genesis.json",
		DataDir:        "",
		BlockGasTarget: "0x0", // Special value signaling the parent gas limit should be applied
		Network: &Network{
			NoDiscover:       defaultNetworkConfig.NoDiscover,
			MaxPeers:         defaultNetworkConfig.MaxPeers,
			MaxOutboundPeers: defaultNetworkConfig.MaxOutboundPeers,
			MaxInboundPeers:  defaultNetworkConfig.MaxInboundPeers,
			Libp2pAddr: fmt.Sprintf("%s:%d",
				defaultNetworkConfig.Addr.IP,
				defaultNetworkConfig.Addr.Port,
			),
		},
		Telemetry:  &Telemetry{},
		ShouldSeal: true,
		TxPool: &TxPool{
			PriceLimit:         0,
			MaxSlots:           4096,
			MaxAccountEnqueued: 128,
		},
		LogLevel:    "INFO",
		RestoreFile: "",
		Headers: &Headers{
			AccessControlAllowOrigins: []string{"*"},
		},
		LogFilePath:              "",
		JSONRPCBatchRequestLimit: DefaultJSONRPCBatchRequestLimit,
		JSONRPCBlockRangeLimit:   DefaultJSONRPCBlockRangeLimit,
		Relayer:                  false,
		NumBlockConfirmations:    DefaultNumBlockConfirmations,
		ConcurrentRequestsDebug:  DefaultConcurrentRequestsDebug,
		WebSocketReadLimit:       DefaultWebSocketReadLimit,
		MetricsInterval:          DefaultMetricsInterval,
	}
}

// ReadConfigFile reads the config file from the specified path, builds a Config object
// and returns it.
//
// Supported file types: .json, .hcl, .yaml, .yml
func ReadConfigFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var unmarshalFunc func([]byte, interface{}) error

	switch {
	case strings.HasSuffix(path, ".hcl"):
		unmarshalFunc = hcl.Unmarshal
	case strings.HasSuffix(path, ".json"):
		unmarshalFunc = json.Unmarshal
	case strings.HasSuffix(path, ".yaml"), strings.HasSuffix(path, ".yml"):
		unmarshalFunc = yaml.Unmarshal
	default:
		return nil, fmt.Errorf("suffix of %s is neither hcl, json, yaml nor yml", path)
	}

	config := DefaultConfig()
	config.Network = new(Network)
	config.Network.MaxPeers = -1
	config.Network.MaxInboundPeers = -1
	config.Network.MaxOutboundPeers = -1

	if err := unmarshalFunc(data, config); err != nil {
		return nil, err
	}

	return config, nil
}
