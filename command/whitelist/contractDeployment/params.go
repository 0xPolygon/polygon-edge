package contractdeployment

import (
	"errors"
	"fmt"
	"os"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/types"
)

const (
	chainFlag         = "chain"
	addAddressFlag    = "addAddress"
	removeAddressFlag = "removeAddress"
)

var (
	errInvalidAddressFormat = errors.New("one or more addresses are invalid format")
	errTypeConversion       = errors.New("invalid conversion")
)

var (
	params = &contractDeploymentParams{}
)

type contractDeploymentParams struct {
	addAddressRaw    []string
	removeAddressRaw []string

	addAddress    []types.Address
	removeAddress []types.Address

	genesisPath   string
	genesisConfig *chain.Chain

	whitelist []types.Address
}

func (p *contractDeploymentParams) initRawParams() error {
	if err := p.initRawAddresses(); err != nil {
		return err
	}

	if err := p.initChain(); err != nil {
		return err
	}

	return nil
}

func (p *contractDeploymentParams) initRawAddresses() error {
	p.addAddress = []types.Address{}
	p.removeAddress = []types.Address{}

	for _, address := range p.addAddressRaw {
		newAddress := types.Address{}

		if err := newAddress.UnmarshalText([]byte(address)); err != nil {
			//TODO add more descriptive error
			return errInvalidAddressFormat
		}

		p.addAddress = append(p.addAddress, newAddress)
	}

	for _, address := range p.removeAddressRaw {
		newAddress := types.Address{}

		if err := newAddress.UnmarshalText([]byte(address)); err != nil {
			//TODO add more descriptive error
			return errInvalidAddressFormat
		}

		p.removeAddress = append(p.removeAddress, newAddress)
	}

	return nil
}

func (p *contractDeploymentParams) initChain() error {
	cc, err := chain.Import(p.genesisPath)
	if err != nil {
		return fmt.Errorf(
			"failed to load chain config from %s: %w",
			p.genesisPath,
			err,
		)
	}

	p.genesisConfig = cc

	return nil
}

func (p *contractDeploymentParams) updateGenesisConfig() error {
	// Fetch whitelist config if exists, if not init
	whitelistConfig := p.genesisConfig.Params.Whitelists
	if len(whitelistConfig) == 0 {
		whitelistConfig = make(map[string]interface{})
	}

	// Extract contract deployment whitelist if exists, if not init
	var contractDeploymentWhitelistRaw []interface{}

	if whitelistConfig["contractDeployment"] != nil {
		var ok bool

		contractDeploymentWhitelistRaw, ok = whitelistConfig["contractDeployment"].([]interface{})
		if !ok {
			return errTypeConversion
		}
	}

	contractDeploymentWhitelist := make([]types.Address, 0)

	for i := range contractDeploymentWhitelistRaw {
		address, ok := contractDeploymentWhitelistRaw[i].(string)
		if !ok {
			return errTypeConversion
		}
		fmt.Println(contractDeploymentWhitelist)
		contractDeploymentWhitelist = append(contractDeploymentWhitelist, types.StringToAddress(address))
	}

	// Add addresses if not exists
	for _, address := range p.addAddress {
		if !address.ExistsIn(contractDeploymentWhitelist) {
			contractDeploymentWhitelist = append(contractDeploymentWhitelist, address)
		}
	}

	newContractDeploymentWhitelist := make([]types.Address, 0)

	// Remove addresses if exists
	for _, address := range contractDeploymentWhitelist {
		if !address.ExistsIn(p.removeAddress) {
			newContractDeploymentWhitelist = append(newContractDeploymentWhitelist, address)
		}
	}

	p.whitelist = newContractDeploymentWhitelist

	whitelistConfig["contractDeployment"] = newContractDeploymentWhitelist
	p.genesisConfig.Params.Whitelists = whitelistConfig

	return nil
}

func (p *contractDeploymentParams) overrideGenesisConfig() error {
	// Remove the current genesis configuration from disk
	if err := os.Remove(p.genesisPath); err != nil {
		return err
	}

	// Save the new genesis configuration
	if err := helper.WriteGenesisConfigToDisk(
		p.genesisConfig,
		p.genesisPath,
	); err != nil {
		return err
	}

	return nil
}

func (p *contractDeploymentParams) getResult() command.CommandResult {
	result := &ContractDeploymentResult{
		AddAddress:    p.addAddress,
		RemoveAddress: p.removeAddress,
		Whitelist:     p.whitelist,
	}

	return result
}
