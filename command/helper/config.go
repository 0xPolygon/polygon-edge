package helper

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"strings"

	"github.com/0xPolygon/polygon-sdk/chain"
	helperFlags "github.com/0xPolygon/polygon-sdk/helper/flags"
	"github.com/0xPolygon/polygon-sdk/secrets"
	"github.com/0xPolygon/polygon-sdk/server"
	"github.com/0xPolygon/polygon-sdk/types"
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
	TxPool         *TxPool                `json:"tx_pool"`
	LogLevel       string                 `json:"log_level"`
	Dev            bool                   `json:"dev_mode"`
	DevInterval    uint64                 `json:"dev_interval"`
	Join           string                 `json:"join_addr"`
	Consensus      map[string]interface{} `json:"consensus"`
}

// Telemetry holds the config details for metric services.
type Telemetry struct {
	PrometheusAddr string `json:"prometheus_addr"`
}

// Network defines the network configuration params
type Network struct {
	NoDiscover bool   `json:"no_discover"`
	Addr       string `json:"libp2p_addr"`
	NatAddr    string `json:"nat_addr"`
	Dns        string `json:"dns_addr"`
	MaxPeers   uint64 `json:"max_peers"`
}

// TxPool defines the TxPool configuration params
type TxPool struct {
	Locals     string `json:"locals"`
	NoLocals   bool   `json:"no_locals"`
	PriceLimit uint64 `json:"price_limit"`
	MaxSlots   uint64 `json:"max_slots"`
}

// DefaultConfig returns the default server configuration
func DefaultConfig() *Config {
	return &Config{
		Chain:          "test",
		DataDir:        "./test-chain",
		BlockGasTarget: "0x0", // Special value signaling the parent gas limit should be applied
		Network: &Network{
			NoDiscover: false,
			MaxPeers:   50,
		},
		Telemetry: &Telemetry{},
		Seal:      false,
		TxPool: &TxPool{
			PriceLimit: 0,
			MaxSlots:   4096,
		},
		Consensus: map[string]interface{}{},
		LogLevel:  "INFO",
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
	conf.DataDir = c.DataDir
	// Set the secrets manager config if it was passed in
	if c.Secrets != "" {
		secretsConfig, readErr := secrets.ReadConfig(c.Secrets)
		if readErr != nil {
			return nil, fmt.Errorf("unable to read config file, %v", readErr)
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
		// If an address was passed in, parse it
		if conf.JSONRPCAddr, err = resolveAddr(c.JSONRPCAddr); err != nil {
			return nil, err
		}
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
				return nil, errors.New("Could not parse NAT IP address")
			}
		}

		if c.Network.Dns != "" {

			if conf.Network.Dns, err = helperFlags.MultiAddrFromDns(c.Network.Dns, conf.Network.Addr.Port); err != nil {
				return nil, err
			}
		}

		conf.Network.NoDiscover = c.Network.NoDiscover
		conf.Network.MaxPeers = c.Network.MaxPeers

		conf.Chain = cc
	}

	// TxPool
	{
		if c.TxPool.Locals != "" {
			strAddrs := strings.Split(c.TxPool.Locals, ",")
			conf.Locals = make([]types.Address, len(strAddrs))
			for i, sAddr := range strAddrs {
				conf.Locals[i] = types.StringToAddress(sAddr)
			}
		}
		conf.NoLocals = c.TxPool.NoLocals
		conf.PriceLimit = c.TxPool.PriceLimit
		conf.MaxSlots = c.TxPool.MaxSlots
	}

	// Target gas limit
	if c.BlockGasTarget != "" {
		value, err := types.ParseUint256orHex(&c.BlockGasTarget)
		if err != nil {
			return nil, fmt.Errorf("failed to parse gas target %s, %v", c.BlockGasTarget, err)
		}
		if !value.IsUint64() {
			return nil, fmt.Errorf("gas target is too large (>64b) %s", c.BlockGasTarget)
		}

		conf.Chain.Params.BlockGasTarget = value.Uint64()
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
		return nil, fmt.Errorf("failed to parse addr '%s': %v", raw, err)
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
		if otherConfig.Network.Dns != "" {
			c.Network.Dns = otherConfig.Network.Dns
		}
		if otherConfig.Network.MaxPeers != 0 {
			c.Network.MaxPeers = otherConfig.Network.MaxPeers
		}
		if otherConfig.Network.NoDiscover {
			c.Network.NoDiscover = true
		}
	}

	if otherConfig.TxPool != nil {
		// TxPool
		if otherConfig.TxPool.Locals != "" {
			c.TxPool.Locals = otherConfig.TxPool.Locals
		}
		if otherConfig.TxPool.NoLocals {
			c.TxPool.NoLocals = otherConfig.TxPool.NoLocals
		}
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
		return nil, fmt.Errorf("Suffix of %s is neither hcl nor json", path)
	}

	var config Config
	if err := unmarshalFunc(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}
