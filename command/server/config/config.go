package config

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/0xPolygon/polygon-edge/network"
	"gopkg.in/yaml.v3"

	"github.com/hashicorp/hcl"
)

// Config defines the server configuration params
type Config struct {
	GenesisPath       string     `json:"chain_config" yaml:"chain_config"`
	SecretsConfigPath string     `json:"secrets_config" yaml:"secrets_config"`
	DataDir           string     `json:"data_dir" yaml:"data_dir"`
	BlockGasTarget    string     `json:"block_gas_target" yaml:"block_gas_target"`
	GRPCAddr          string     `json:"grpc_addr" yaml:"grpc_addr"`
	JSONRPCAddr       string     `json:"jsonrpc_addr" yaml:"jsonrpc_addr"`
	Telemetry         *Telemetry `json:"telemetry" yaml:"telemetry"`
	Network           *Network   `json:"network" yaml:"network"`
	ShouldSeal        bool       `json:"seal" yaml:"seal"`
	TxPool            *TxPool    `json:"tx_pool" yaml:"tx_pool"`
	LogLevel          string     `json:"log_level" yaml:"log_level"`
	RestoreFile       string     `json:"restore_file" yaml:"restore_file"`
	BlockTime         uint64     `json:"block_time_s" yaml:"block_time_s"`
	IBFTBaseTimeout   uint64     `json:"ibft_base_time_s" yaml:"ibft_base_time_s"`
	Headers           *Headers   `json:"headers" yaml:"headers"`
	LogFilePath       string     `json:"log_to" yaml:"log_to"`
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
	PriceLimit uint64 `json:"price_limit" yaml:"price_limit"`
	MaxSlots   uint64 `json:"max_slots" yaml:"max_slots"`
}

// Headers defines the HTTP response headers required to enable CORS.
type Headers struct {
	AccessControlAllowOrigins []string `json:"access_control_allow_origins" yaml:"access_control_allow_origins"`
}

const (
	// minimum block generation time in seconds
	DefaultBlockTime uint64 = 2

	// IBFT timeout in seconds
	DefaultIBFTBaseTimeout uint64 = 10

	// Multiplier to get IBFT timeout from block time
	// timeout is calculated when IBFT timeout is not specified
	BlockTimeMultiplierForTimeout uint64 = 5
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
			PriceLimit: 0,
			MaxSlots:   4096,
		},
		LogLevel:        "INFO",
		RestoreFile:     "",
		BlockTime:       DefaultBlockTime,
		IBFTBaseTimeout: DefaultIBFTBaseTimeout,
		Headers: &Headers{
			AccessControlAllowOrigins: []string{"*"},
		},
		LogFilePath: "",
	}
}

// ReadConfigFile reads the config file from the specified path, builds a Config object
// and returns it.
//
//Supported file types: .json, .hcl, .yaml, .yml
func ReadConfigFile(path string) (*Config, error) {
	data, err := ioutil.ReadFile(path)
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
