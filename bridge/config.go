package bridge

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"os"
)

const (
	// TODO: should be given from command line option
	ConfigFileName = "chainbridge.config.json"
)

type Config struct {
	Genesis      GenesisConfig            `json:"genesis"`
	Destinations []DestinationChainConfig `json:"destinations"`
}

type GenesisConfig struct {
	Admin            string   `json:"admin"`
	Relayers         []string `json:"relayers"`
	RelayerThreshold int64    `json:"relayer_threshold"`
}

type DestinationChainConfig struct {
	Type     string            `json:"type"`
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Endpoint string            `json:"endpoint"`
	Opts     map[string]string `json:"opts"`
}

func LoadConfig() (*Config, error) {
	path := ConfigFileName
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, errors.New("doesn't exist")
	}
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	config := &Config{}
	if err := json.Unmarshal(data, config); err != nil {
		return nil, err
	}
	return config, nil
}
