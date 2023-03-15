package service

import (
	"strings"
	"time"

	"github.com/0xPolygon/polygon-edge/types"
)

type AAConfig struct {
	AllowContractCreation bool          `json:"allowContractCreation"`
	AllowList             []string      `json:"allowList"`
	DenyList              []string      `json:"denyList"`
	PullTime              time.Duration `json:"pullTime"`
	ReceiptRetryDelay     time.Duration `json:"receiptRetryDelay"`
	ReceiptNumRetries     int           `json:"receiptNumRetries"`
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
		PullTime:              time.Millisecond * 2000, // every five seconds pull from pool
		ReceiptRetryDelay:     time.Millisecond * 500,
		ReceiptNumRetries:     100,
	}
}
