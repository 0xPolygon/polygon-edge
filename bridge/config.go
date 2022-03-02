package bridge

import (
	"net/url"

	"github.com/0xPolygon/polygon-edge/bridge/tracker"
	"github.com/0xPolygon/polygon-edge/types"
)

type Config struct {
	Enable            bool
	RootChainURL      *url.URL
	RootChainContract types.Address
	Confirmations     uint64
}

func DefaultConfig() *Config {
	return &Config{
		Enable:            false,
		RootChainURL:      nil,
		RootChainContract: types.ZeroAddress,
		Confirmations:     tracker.DefaultBlockConfirmations,
	}
}
