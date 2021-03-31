package server

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"strings"

	"github.com/0xPolygon/minimal/chain"
	"github.com/0xPolygon/minimal/minimal"
	"github.com/hashicorp/hcl"
	"github.com/imdario/mergo"
)

type Config struct {
	Chain       string                 `json:"chain"`
	DataDir     string                 `json:"data_dir"`
	GRPCAddr    string                 `json:"rpc_addr"`
	JSONRPCAddr string                 `json:"jsonrpc_addr"`
	Network     *Network               `json:"network"`
	Telemetry   *Telemetry             `json:"telemetry"`
	Seal        bool                   `json:"seal"`
	LogLevel    string                 `json:"log_level"`
	Consensus   map[string]interface{} `json:"consensus"`
	Dev         bool
	Join        string
}

type Network struct {
	NoDiscover bool   `json:"no_discover"`
	Addr       string `json:"addr"`
	MaxPeers   uint64 `json:"max_peers"`
}

type Telemetry struct {
	PrometheusPort int `json:"prometheus_port"`
}

func defaultConfig() *Config {
	return &Config{
		Chain:   "test",
		DataDir: "./test-chain",
		Telemetry: &Telemetry{
			PrometheusPort: 8080,
		},
		Network: &Network{
			NoDiscover: false,
			MaxPeers:   20,
		},
		Seal:      false,
		LogLevel:  "INFO",
		Consensus: map[string]interface{}{},
	}
}

func (c *Config) BuildConfig() (*minimal.Config, error) {
	conf := minimal.DefaultConfig()

	// decode chain
	chain, err := chain.Import(c.Chain)
	if err != nil {
		return nil, err
	}
	conf.Chain = chain
	conf.Seal = c.Seal
	conf.DataDir = c.DataDir

	if c.GRPCAddr != "" {
		if conf.GRPCAddr, err = resolveAddr(c.GRPCAddr); err != nil {
			return nil, err
		}
	}
	if c.JSONRPCAddr != "" {
		if conf.JSONRPCAddr, err = resolveAddr(c.JSONRPCAddr); err != nil {
			return nil, err
		}
	}
	// network
	{
		if conf.Network.Addr, err = resolveAddr(c.Network.Addr); err != nil {
			return nil, err
		}
		conf.Network.NoDiscover = c.Network.NoDiscover
		conf.Network.MaxPeers = c.Network.MaxPeers
		conf.Chain = chain
	}

	// if we are in dev mode, change the consensus protocol with 'dev'
	// and disable discovery of other nodes
	// TODO: Disable networking altogheter.
	if c.Dev {
		conf.Network.NoDiscover = true
		conf.Chain.Params.Engine = map[string]interface{}{
			"dev": nil,
		}
	}

	return conf, nil
}

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

func (c *Config) merge(c1 *Config) error {
	if c1.DataDir != "" {
		c.DataDir = c1.DataDir
	}
	if c1.Chain != "" {
		c.Chain = c1.Chain
	}
	if c1.Dev {
		c.Dev = true
	}
	if c1.Telemetry != nil {
		if c1.Telemetry.PrometheusPort != 0 {
			c.Telemetry.PrometheusPort = c1.Telemetry.PrometheusPort
		}
	}
	if c1.Seal {
		c.Seal = true
	}
	if c1.LogLevel != "" {
		c.LogLevel = c1.LogLevel
	}
	if c1.GRPCAddr != "" {
		c.GRPCAddr = c1.GRPCAddr
	}
	if c1.JSONRPCAddr != "" {
		c.JSONRPCAddr = c1.JSONRPCAddr
	}
	if c1.Join != "" {
		c.Join = c1.Join
	}
	{
		// network
		if c1.Network.Addr != "" {
			c.Network.Addr = c1.Network.Addr
		}
		if c1.Network.MaxPeers != 0 {
			c.Network.MaxPeers = c1.Network.MaxPeers
		}
		if c1.Network.NoDiscover {
			c.Network.NoDiscover = true
		}
	}
	if err := mergo.Merge(&c.Consensus, c1.Consensus, mergo.WithOverride); err != nil {
		return err
	}
	return nil
}

func readConfigFile(path string) (*Config, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var f func([]byte, interface{}) error
	switch {
	case strings.HasSuffix(path, ".hcl"):
		f = hcl.Unmarshal
	case strings.HasSuffix(path, ".json"):
		f = json.Unmarshal
	default:
		return nil, fmt.Errorf("Suffix of %s is neither hcl nor json", path)
	}

	var config Config
	if err := f(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}
