package agent

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/hashicorp/hcl"
)

type Config struct {
	Chain     string `json:"chain"`
	DataDir   string `json:"data-dir"`
	BindAddr  string `json:"addr"`
	BindPort  int    `json:"port"`
	Telemetry *Telemetry
}

type Telemetry struct {
	PrometheusPort int
}

func DefaultTelemetry() *Telemetry {
	return &Telemetry{
		PrometheusPort: 8080,
	}
}

func DefaultConfig() *Config {
	return &Config{
		Chain:     "foundation",
		DataDir:   "./test-chain",
		BindAddr:  "0.0.0.0",
		BindPort:  30303,
		Telemetry: DefaultTelemetry(),
	}
}

func (c *Config) merge(c1 *Config) {
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
}

// Merge merges configurations
func (c *Config) Merge(c1 ...*Config) {
	for _, i := range c1 {
		c.merge(i)
	}
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
