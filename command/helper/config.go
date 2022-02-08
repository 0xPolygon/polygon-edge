package helper

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"strings"

	"github.com/0xPolygon/polygon-edge/chain"
	helperFlags "github.com/0xPolygon/polygon-edge/helper/flags"
	"github.com/0xPolygon/polygon-edge/secrets"
	"github.com/0xPolygon/polygon-edge/server"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/hcl"
	"github.com/imdario/mergo"
)

// Config defines the server configuration params
type Config struct {
	Chain          string                 `json:"chain_config"`
	Secrets        string                 `json:"secrets_config"`
	DataDir        string                 `json:"data_dir"`
	BlockGasTarget string                 `json:"block_gas_target"`
	GRPCAddr       string                 `json:"grpc_addr"`
	JSONRPCAddr    string                 `json:"jsonrpc_addr"`
	Telemetry      *Telemetry             `json:"telemetry"`
	Network        *Network               `json:"network"`
	Seal           bool                   `json:"seal"`
	Tracker        bool                   `json:"tracker"`
	TxPool         *TxPool                `json:"tx_pool"`
	LogLevel       string                 `json:"log_level"`
	Dev            bool                   `json:"dev_mode"`
	DevInterval    uint64                 `json:"dev_interval"`
	Join           string                 `json:"join_addr"`
	Consensus      map[string]interface{} `json:"consensus"`
	Headers        *Headers               `json:"headers"`
	RestoreFile    string                 `json:"restore_file"`
	BlockTime      uint64                 `json:"block_time_s"`
}

// Telemetry holds the config details for metric services.
type Telemetry struct {
	PrometheusAddr string `json:"prometheus_addr"`
}

// Network defines the network configuration params
type Network struct {
	NoDiscover       bool   `json:"no_discover"`
	Addr             string `json:"libp2p_addr"`
	NatAddr          string `json:"nat_addr"`
	DNS              string `json:"dns_addr"`
	MaxPeers         int64  `json:"max_peers,omitempty"`
	MaxOutboundPeers int64  `json:"max_outbound_peers,omitempty"`
	MaxInboundPeers  int64  `json:"max_inbound_peers,omitempty"`
}

// TxPool defines the TxPool configuration params
type TxPool struct {
	PriceLimit uint64 `json:"price_limit"`
	MaxSlots   uint64 `json:"max_slots"`
}

// Headers defines the HTTP response headers required to enable CORS.
type Headers struct {
	AccessControlAllowOrigins []string `json:"access_control_allow_origins"`
}

// minimum block generation time in seconds
const defaultBlockTime uint64 = 2

// DefaultConfig returns the default server configuration
func DefaultConfig() *Config {
	return &Config{
		Chain:          "test",
		DataDir:        "./test-chain",
		BlockGasTarget: "0x0", // Special value signaling the parent gas limit should be applied
		Network: &Network{
			NoDiscover:       false,
			MaxPeers:         40,
			MaxOutboundPeers: 8,
			MaxInboundPeers:  32,
		},
		Telemetry: &Telemetry{},
		Seal:      false,
		Tracker:   false,
		TxPool: &TxPool{
			PriceLimit: 0,
			MaxSlots:   4096,
		},
		Consensus:   map[string]interface{}{},
		LogLevel:    "INFO",
		RestoreFile: "",
		BlockTime:   defaultBlockTime,
	}
}

// BuildConfig Builds the config based on set parameters
func (c *Config) BuildConfig() (*server.Config, error) {
	// Grab the default server config
	conf := server.DefaultConfig()

	// Decode the chain
	cc, err := chain.Import(c.Chain)
	if err != nil {
		return nil, err
	}

	conf.Chain = cc
	conf.Seal = c.Seal
	conf.Tracker = c.Tracker
	conf.DataDir = c.DataDir
	// Set the secrets manager config if it was passed in
	if c.Secrets != "" {
		secretsConfig, readErr := secrets.ReadConfig(c.Secrets)
		if readErr != nil {
			return nil, fmt.Errorf("unable to read config file, %w", readErr)
		}

		conf.SecretsManager = secretsConfig
	}
	// JSON RPC + GRPC
	if c.GRPCAddr != "" {
		// If an address was passed in, parse it
		if conf.GRPCAddr, err = resolveAddr(c.GRPCAddr); err != nil {
			return nil, err
		}
	}

	if c.JSONRPCAddr != "" {
		if conf.JSONRPC.JSONRPCAddr, err = resolveAddr(c.JSONRPCAddr); err != nil {
			return nil, err
		}
	}

	if c.Headers != nil {
		conf.JSONRPC.AccessControlAllowOrigin = c.Headers.AccessControlAllowOrigins
	}

	if c.Telemetry.PrometheusAddr != "" {
		// If an address was passed in, parse it
		if conf.Telemetry.PrometheusAddr, err = resolveAddr(c.Telemetry.PrometheusAddr); err != nil {
			return nil, err
		}
	}

	// Network
	{
		if conf.Network.Addr, err = resolveAddr(c.Network.Addr); err != nil {
			return nil, err
		}

		if c.Network.NatAddr != "" {
			if conf.Network.NatAddr = net.ParseIP(c.Network.NatAddr); conf.Network.NatAddr == nil {
				return nil, errors.New("could not parse NAT IP address")
			}
		}

		if c.Network.DNS != "" {
			if conf.Network.DNS, err = helperFlags.MultiAddrFromDNS(c.Network.DNS, conf.Network.Addr.Port); err != nil {
				return nil, err
			}
		}
		conf.Network.NoDiscover = c.Network.NoDiscover
		conf.Network.MaxPeers = c.Network.MaxPeers
		conf.Network.MaxInboundPeers = c.Network.MaxInboundPeers
		conf.Network.MaxOutboundPeers = c.Network.MaxOutboundPeers

		conf.Chain = cc
	}

	// TxPool
	{
		conf.PriceLimit = c.TxPool.PriceLimit
		conf.MaxSlots = c.TxPool.MaxSlots
	}

	// Target gas limit
	if c.BlockGasTarget != "" {
		value, err := types.ParseUint256orHex(&c.BlockGasTarget)
		if err != nil {
			return nil, fmt.Errorf("failed to parse gas target %s, %w", c.BlockGasTarget, err)
		}

		if !value.IsUint64() {
			return nil, fmt.Errorf("gas target is too large (>64b) %s", c.BlockGasTarget)
		}

		conf.Chain.Params.BlockGasTarget = value.Uint64()
	}

	if c.RestoreFile != "" {
		conf.RestoreFile = &c.RestoreFile
	}

	// set block time if not default
	if c.BlockTime != defaultBlockTime {
		conf.BlockTime = c.BlockTime
	}

	// if we are in dev mode, change the consensus protocol with 'dev'
	// and disable discovery of other nodes
	// TODO: Disable networking altogether.
	if c.Dev {
		conf.Seal = true
		conf.Network.NoDiscover = true

		engineConfig := map[string]interface{}{}
		if c.DevInterval != 0 {
			engineConfig["interval"] = c.DevInterval
		}

		conf.Chain.Params.Forks = chain.AllForksEnabled
		conf.Chain.Params.Engine = map[string]interface{}{
			"dev": engineConfig,
		}
	}

	return conf, nil
}

// resolveAddr resolves the passed in TCP address
func resolveAddr(raw string) (*net.TCPAddr, error) {
	addr, err := net.ResolveTCPAddr("tcp", raw)

	if err != nil {
		return nil, fmt.Errorf("failed to parse addr '%s': %w", raw, err)
	}

	if addr.IP == nil {
		addr.IP = net.ParseIP("127.0.0.1")
	}

	return addr, nil
}

// mergeConfigWith merges the passed in configuration to the current configuration
func (c *Config) mergeConfigWith(otherConfig *Config) error {
	if otherConfig.DataDir != "" {
		c.DataDir = otherConfig.DataDir
	}

	if otherConfig.Chain != "" {
		c.Chain = otherConfig.Chain
	}

	if otherConfig.Dev {
		c.Dev = true
	}

	if otherConfig.DevInterval != 0 {
		c.DevInterval = otherConfig.DevInterval
	}

	if otherConfig.BlockGasTarget != "" {
		c.BlockGasTarget = otherConfig.BlockGasTarget
	}

	if otherConfig.Seal {
		c.Seal = true
	}

	if otherConfig.Tracker {
		c.Tracker = true
	}

	if otherConfig.LogLevel != "" {
		c.LogLevel = otherConfig.LogLevel
	}

	if otherConfig.GRPCAddr != "" {
		c.GRPCAddr = otherConfig.GRPCAddr
	}

	if otherConfig.Telemetry != nil {
		if otherConfig.Telemetry.PrometheusAddr != "" {
			c.Telemetry.PrometheusAddr = otherConfig.Telemetry.PrometheusAddr
		}
	}

	if otherConfig.JSONRPCAddr != "" {
		c.JSONRPCAddr = otherConfig.JSONRPCAddr
	}

	if otherConfig.Headers != nil {
		c.Headers = otherConfig.Headers
	}

	if otherConfig.Join != "" {
		c.Join = otherConfig.Join
	}

	if otherConfig.Network != nil {
		// Network
		if otherConfig.Network.Addr != "" {
			c.Network.Addr = otherConfig.Network.Addr
		}

		if otherConfig.Network.NatAddr != "" {
			c.Network.NatAddr = otherConfig.Network.NatAddr
		}

		if otherConfig.Network.DNS != "" {
			c.Network.DNS = otherConfig.Network.DNS
		}

		if otherConfig.Network.MaxPeers > -1 {
			c.Network.MaxPeers = otherConfig.Network.MaxPeers
		}

		if otherConfig.Network.MaxInboundPeers > -1 {
			c.Network.MaxInboundPeers = otherConfig.Network.MaxInboundPeers
		}

		if otherConfig.Network.MaxOutboundPeers > -1 {
			c.Network.MaxOutboundPeers = otherConfig.Network.MaxOutboundPeers
		}

		if otherConfig.Network.NoDiscover {
			c.Network.NoDiscover = true
		}
	}

	if otherConfig.TxPool != nil {
		// TxPool
		if otherConfig.TxPool.PriceLimit != 0 {
			c.TxPool.PriceLimit = otherConfig.TxPool.PriceLimit
		}

		if otherConfig.TxPool.MaxSlots != 0 {
			c.TxPool.MaxSlots = otherConfig.TxPool.MaxSlots
		}
	}
	// Read the secrets config file location
	if otherConfig.Secrets != "" {
		c.Secrets = otherConfig.Secrets
	}

	if otherConfig.RestoreFile != "" {
		c.RestoreFile = otherConfig.RestoreFile
	}

	// if block time not default, set to new value
	if otherConfig.BlockTime != defaultBlockTime {
		c.BlockTime = otherConfig.BlockTime
	}

	if err := mergo.Merge(&c.Consensus, otherConfig.Consensus, mergo.WithOverride); err != nil {
		return err
	}

	return nil
}

// readConfigFile reads the config file from the specified path, builds a Config object
// and returns it.
//
//Supported file types: .json, .hcl
func readConfigFile(path string) (*Config, error) {
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
	default:
		return nil, fmt.Errorf("suffix of %s is neither hcl nor json", path)
	}

	config := new(Config)
	config.Network = new(Network)
	config.Network.MaxPeers = -1
	config.Network.MaxInboundPeers = -1
	config.Network.MaxOutboundPeers = -1

	if err := unmarshalFunc(data, config); err != nil {
		return nil, err
	}

	return config, nil
}
