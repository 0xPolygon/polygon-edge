package service

import (
	"embed"
	"encoding/json"

	"github.com/0xPolygon/polygon-edge/types"
)

//go:embed config/*
var configFile embed.FS

type AAConfig struct {
	Whitelist []string `json:"whitelist"`
	Blacklist []string `json:"blacklist"`
}

func (c *AAConfig) IsValidAddress(address *types.Address) bool {
	if address == nil {
		return true // TODO: contract creation is ok?
	}

	str := address.String()[2:] // skip 0x

	for _, v := range c.Blacklist {
		if str == v {
			return false
		}
	}

	if len(c.Whitelist) == 0 {
		return true
	}

	for _, v := range c.Whitelist {
		if str == v {
			return true
		}
	}

	return false
}

func GetConfig() (*AAConfig, error) {
	bytes, err := configFile.ReadFile("config/config.json")
	if err != nil {
		return nil, err
	}

	config := &AAConfig{}

	if err := json.Unmarshal(bytes, &config); err != nil {
		return nil, err
	}

	return config, nil
}
