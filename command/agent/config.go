package agent

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/hashicorp/hcl"
	"github.com/imdario/mergo"
)

type Config struct {
	Chain       string     `json:"chain"`
	DataDir     string     `json:"data_dir"`
	BindAddr    string     `json:"bind_addr"`
	BindPort    int        `json:"bind_port"`
	Telemetry   *Telemetry `json:"telemetry"`
	ServiceName string     `json:"service_name"`
	Seal        bool       `json:"seal"`

	Blockchain *BlockchainConfig        `json:"blockchain"`
	Protocols  map[string]BackendConfig `json:"protocols"`
	Discovery  map[string]BackendConfig `json:"discovery"`
	Consensus  BackendConfig            `json:"consensus"`
}

type Telemetry struct {
	PrometheusPort int `json:"prometheus_port"`
}

func DefaultConfig() *Config {
	return &Config{
		Chain:       "foundation",
		DataDir:     "./test-chain",
		BindAddr:    "0.0.0.0",
		BindPort:    30303,
		ServiceName: "minimal",
		Telemetry: &Telemetry{
			PrometheusPort: 8080,
		},
		Seal: false,
		Blockchain: &BlockchainConfig{
			Backend: "leveldb",
		},
		Protocols: map[string]BackendConfig{},
		Discovery: map[string]BackendConfig{},
		Consensus: BackendConfig{},
	}
}

func (c *Config) merge(c1 *Config) error {
	if c1.BindAddr != "" {
		c.BindAddr = c1.BindAddr
	}
	if c1.BindPort != 0 {
		c.BindPort = c1.BindPort
	}
	if c1.DataDir != "" {
		c.DataDir = c1.DataDir
	}
	if c1.Chain != "" {
		c.Chain = c1.Chain
	}
	if c1.ServiceName != "" {
		c.ServiceName = c1.ServiceName
	}
	if c1.Telemetry != nil {
		if c1.Telemetry.PrometheusPort != 0 {
			c.Telemetry.PrometheusPort = c1.Telemetry.PrometheusPort
		}
	}
	if c1.Seal {
		c.Seal = true
	}
	if err := mergo.Merge(&c.Protocols, c1.Protocols, mergo.WithOverride); err != nil {
		return err
	}
	if err := mergo.Merge(&c.Discovery, c1.Discovery, mergo.WithOverride); err != nil {
		return err
	}
	if err := mergo.Merge(&c.Consensus, c1.Consensus, mergo.WithOverride); err != nil {
		return err
	}
	if c.Blockchain != nil && c1.Blockchain != nil {
		if err := mergo.Merge(&c.Blockchain, c1.Blockchain); err != nil {
			return err
		}
	}
	return nil
}

// Merge merges configurations
func (c *Config) Merge(c1 ...*Config) error {
	for _, i := range c1 {
		if err := c.merge(i); err != nil {
			return err
		}
	}
	return nil
}

// BackendConfig is the configuration for the backends
type BackendConfig map[string]interface{}

// BlockchainConfig is the blockchain configuration
type BlockchainConfig struct {
	Backend string        `json:"backend"`
	Config  BackendConfig `json:"config"`
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
