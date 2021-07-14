package helper

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"strings"

	"github.com/0xPolygon/minimal/chain"
	"github.com/0xPolygon/minimal/minimal"
	"github.com/hashicorp/hcl"
	"github.com/imdario/mergo"
)

// Config defines the server configuration params
type Config struct {
	Chain       string                 `json:"chain"`
	DataDir     string                 `json:"data_dir"`
	GRPCAddr    string                 `json:"rpc_addr"`
	JSONRPCAddr string                 `json:"jsonrpc_addr"`
	Network     *Network               `json:"network"`
	Seal        bool                   `json:"seal"`
	LogLevel    string                 `json:"log_level"`
	Consensus   map[string]interface{} `json:"consensus"`
	Dev         bool
	DevInterval uint64
	Join        string
}

// Network defines the network configuration params
type Network struct {
	NoDiscover bool   `json:"no_discover"`
	Addr       string `json:"addr"`
	NatAddr    string `json:"nat_addr"`
	MaxPeers   uint64 `json:"max_peers"`
}

// DefaultConfig returns the default server configuration
func DefaultConfig() *Config {
	return &Config{
		Chain:   "test",
		DataDir: "./test-chain",
		Network: &Network{
			NoDiscover: false,
			MaxPeers:   20,
		},
		Seal:      false,
		LogLevel:  "INFO",
		Consensus: map[string]interface{}{},
	}
}

// BuildConfig Builds the config based on set parameters
func (c *Config) BuildConfig() (*minimal.Config, error) {
	// Grab the default Minimal server config
	conf := minimal.DefaultConfig()

	// Decode the chain
	cc, err := chain.Import(c.Chain)
	if err != nil {
		return nil, err
	}

	conf.Chain = cc
	conf.Seal = c.Seal
	conf.DataDir = c.DataDir

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

		conf.Network.NoDiscover = c.Network.NoDiscover
		conf.Network.MaxPeers = c.Network.MaxPeers

		conf.Chain = cc
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

	if otherConfig.Seal {
		c.Seal = true
	}

	if otherConfig.LogLevel != "" {
		c.LogLevel = otherConfig.LogLevel
	}

	if otherConfig.GRPCAddr != "" {
		c.GRPCAddr = otherConfig.GRPCAddr
	}

	if otherConfig.JSONRPCAddr != "" {
		c.JSONRPCAddr = otherConfig.JSONRPCAddr
	}

	if otherConfig.Join != "" {
		c.Join = otherConfig.Join
	}

	{
		// Network
		if otherConfig.Network.Addr != "" {
			c.Network.Addr = otherConfig.Network.Addr
		}
		if otherConfig.Network.NatAddr != "" {
			c.Network.NatAddr = otherConfig.Network.NatAddr
		}
		if otherConfig.Network.MaxPeers != 0 {
			c.Network.MaxPeers = otherConfig.Network.MaxPeers
		}
		if otherConfig.Network.NoDiscover {
			c.Network.NoDiscover = true
		}
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
