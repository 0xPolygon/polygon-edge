package discv4

import (
	"github.com/mitchellh/mapstructure"
)

// Config is the discover configuration
type Config struct {
	BindAddr  string   `mapstructure:"addr"`
	BindPort  int      `mapstructure:"port"`
	Bootnodes []string `mapstructure:"bootnodes"`
}

func (c *Config) merge(c1 *Config) {
	if c1.BindAddr != "" {
		c.BindAddr = c1.BindAddr
	}
	if c1.BindPort != 0 {
		c.BindPort = c1.BindPort
	}
	if len(c1.Bootnodes) != 0 {
		c.Bootnodes = c1.Bootnodes
	}
}

// DefaultConfig is the default config
func DefaultConfig() *Config {
	c := &Config{
		BindAddr:  "0.0.0.0",
		BindPort:  30303,
		Bootnodes: []string{},
	}
	return c
}

func readConfig(conf map[string]interface{}) (*Config, error) {
	defConfig := DefaultConfig()

	var config *Config
	if err := mapstructure.Decode(conf, &config); err != nil {
		return nil, err
	}

	defConfig.merge(config)
	return defConfig, nil
}
