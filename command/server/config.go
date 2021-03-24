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
	LibP2PAddr  string                 `json:"bind_addr"`
	GRPCAddr    string                 `json:"rpc_addr"`
	JSONRPCAddr string                 `json:"jsonrpc_addr"`
	Telemetry   *Telemetry             `json:"telemetry"`
	Seal        bool                   `json:"seal"`
	LogLevel    string                 `json:"log_level"`
	Consensus   map[string]interface{} `json:"consensus"`
	Join        string
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
		Seal:      false,
		LogLevel:  "INFO",
		Consensus: map[string]interface{}{},
	}
}

func (c *Config) BuildConfig() (*minimal.Config, error) {
	conf := minimal.DefaultConfig()

	// decode chain
	chain, err := chain.ImportFromName(c.Chain)
	if err != nil {
		return nil, err
	}
	conf.Chain = chain
	conf.Seal = c.Seal
	conf.DataDir = c.DataDir

	if c.LibP2PAddr != "" {
		addr, err := net.ResolveTCPAddr("tcp", c.LibP2PAddr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse libP2P addr '%s': %v", c.LibP2PAddr, err)
		}
		if addr.IP == nil {
			addr.IP = net.ParseIP("127.0.0.1")
		}
		conf.LibP2PAddr = addr
	}
	if c.GRPCAddr != "" {
		addr, err := net.ResolveTCPAddr("tcp", c.GRPCAddr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse grpc addr '%s': %v", c.GRPCAddr, err)
		}
		conf.GRPCAddr = addr
	}
	if c.JSONRPCAddr != "" {
		addr, err := net.ResolveTCPAddr("tcp", c.JSONRPCAddr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse jsonrc addr '%s': %v", c.JSONRPCAddr, err)
		}
		conf.JSONRPCAddr = addr
	}
	return conf, nil
}

func (c *Config) merge(c1 *Config) error {
	if c1.DataDir != "" {
		c.DataDir = c1.DataDir
	}
	if c1.Chain != "" {
		c.Chain = c1.Chain
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
	if c1.LibP2PAddr != "" {
		c.LibP2PAddr = c1.LibP2PAddr
	}
	if c1.JSONRPCAddr != "" {
		c.JSONRPCAddr = c1.JSONRPCAddr
	}
	if c1.Join != "" {
		c.Join = c1.Join
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
