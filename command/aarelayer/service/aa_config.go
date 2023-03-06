package service

import (
	"embed"
	"encoding/json"
	"strings"

	"github.com/0xPolygon/polygon-edge/types"
)

//go:embed config/*
var configFile embed.FS

type AAConfig struct {
	AllowContractCreation bool     `json:"allowContractCreation"`
	AllowList             []string `json:"allowList"`
	DenyList              []string `json:"denyList"`
}

func (c *AAConfig) IsValidAddress(address *types.Address) bool {
	if address == nil {
		return c.AllowContractCreation
	}

	addressStr := strings.TrimPrefix(address.String(), "0x")

	for _, v := range c.DenyList {
		if addressStr == strings.TrimPrefix(v, "0x") {
			return false
		}
	}

	if len(c.AllowList) == 0 {
		return true
	}

	for _, v := range c.AllowList {
		if addressStr == strings.TrimPrefix(v, "0x") {
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
