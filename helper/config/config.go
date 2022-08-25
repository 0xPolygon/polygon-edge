package config

import (
	"errors"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/types"
)

const (
	// DeploymentWhitelistKey is the key used for the deployment whitelist
	DeploymentWhitelistKey = "deployment"
)

var (
	ErrAddressTypeAssertion   = errors.New("invalid type assertion for address")
	ErrWhitelistTypeAssertion = errors.New("invalid type assertion for deployment whitelist")
)

// GetWhitelist fetches whitelist object from the config
// if doesn't exist returns empty map
func GetWhitelist(config *chain.Chain) map[string]interface{} {
	// Fetch whitelist if exists, if not init
	whitelistConfig := config.Params.Whitelists
	if len(whitelistConfig) == 0 {
		whitelistConfig = make(map[string]interface{})
	}

	return whitelistConfig
}

// GetDeploymentWhitelist fetches deployment whitelist from the genesis config
// if doesn't exist returns empty list
func GetDeploymentWhitelist(genesisConfig *chain.Chain) ([]types.Address, error) {
	// Fetch whitelist config if exists, if not init
	whitelistConfig := GetWhitelist(genesisConfig)

	// Extract deployment whitelist if exists, if not init

	var deploymentWhitelistRaw []interface{}

	if whitelistConfig[DeploymentWhitelistKey] != nil {
		var ok bool

		deploymentWhitelistRaw, ok = whitelistConfig[DeploymentWhitelistKey].([]interface{})
		if !ok {
			return nil, ErrAddressTypeAssertion
		}
	}

	deploymentWhitelist := make([]types.Address, 0, len(deploymentWhitelistRaw))

	for i := range deploymentWhitelistRaw {
		address, ok := deploymentWhitelistRaw[i].(string)
		if !ok {
			return nil, ErrWhitelistTypeAssertion
		}

		deploymentWhitelist = append(deploymentWhitelist, types.StringToAddress(address))
	}

	return deploymentWhitelist, nil
}
