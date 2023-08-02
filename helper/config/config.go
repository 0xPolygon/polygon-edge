package config

import (
	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/types"
)

// GetWhitelist fetches whitelist object from the config
func GetWhitelist(config *chain.Chain) *chain.Whitelists {
	return config.Params.Whitelists
}

// GetDeploymentWhitelist fetches deployment whitelist from the genesis config
// if doesn't exist returns empty list
func GetDeploymentWhitelist(genesisConfig *chain.Chain) ([]types.Address, error) {
	// Fetch whitelist config if exists, if not init
	whitelistConfig := GetWhitelist(genesisConfig)

	// Extract deployment whitelist if exists, if not init
	if whitelistConfig == nil {
		return make([]types.Address, 0), nil
	}

	return whitelistConfig.Deployment, nil
}
