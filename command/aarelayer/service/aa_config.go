package service

import (
	"strings"

	"github.com/0xPolygon/polygon-edge/types"
)

type AAConfig struct {
	AllowContractCreation bool     `json:"allowContractCreation"`
	AllowList             []string `json:"allowList"`
	DenyList              []string `json:"denyList"`
}

func (c *AAConfig) IsValidAddress(address types.Address) bool {
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

func DefaultConfig() *AAConfig {
	return &AAConfig{
		AllowContractCreation: true,
		AllowList:             nil,
		DenyList:              nil,
	}
}
