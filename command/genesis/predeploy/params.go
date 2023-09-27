package predeploy

import (
	"errors"
	"fmt"
	"os"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/contracts/staking"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/helper/predeployment"
	"github.com/0xPolygon/polygon-edge/types"
)

const (
	chainFlag            = "chain"
	predeployAddressFlag = "predeploy-address"
	artifactsPathFlag    = "artifacts-path"
	constructorArgsPath  = "constructor-args"
)

var (
	errInvalidPredeployAddress  = errors.New("invalid predeploy address provided")
	errAddressTaken             = errors.New("the provided predeploy address is taken")
	errReservedPredeployAddress = errors.New("the provided predeploy address is reserved")
	errInvalidAddress           = fmt.Errorf(
		"the provided predeploy address must be >= %s", predeployAddressMin.String(),
	)
)

var (
	predeployAddressMin = types.StringToAddress("01100")
	reservedAddresses   = []types.Address{
		staking.AddrStakingContract,
	}
)

var (
	params = &predeployParams{}
)

type predeployParams struct {
	addressRaw  string
	genesisPath string

	address         types.Address
	artifactsPath   string
	constructorArgs []string

	genesisConfig *chain.Chain
}

func (p *predeployParams) getRequiredFlags() []string {
	return []string{
		predeployAddressFlag,
		artifactsPathFlag,
	}
}

func (p *predeployParams) initRawParams() error {
	if err := p.initPredeployAddress(); err != nil {
		return err
	}

	if err := p.verifyMinAddress(); err != nil {
		return err
	}

	if err := p.initChain(); err != nil {
		return err
	}

	return nil
}

func (p *predeployParams) initPredeployAddress() error {
	if p.addressRaw == "" {
		return errInvalidPredeployAddress
	}

	address := types.StringToAddress(p.addressRaw)
	if isReservedAddress(address) {
		return errReservedPredeployAddress
	}

	p.address = address

	return nil
}

func isReservedAddress(address types.Address) bool {
	for _, reservedAddress := range reservedAddresses {
		if address == reservedAddress {
			return true
		}
	}

	return false
}

func (p *predeployParams) verifyMinAddress() error {
	address, err := hex.DecodeHexToBig(p.address.String())
	if err != nil {
		return err
	}

	addressMin, err := hex.DecodeHexToBig(predeployAddressMin.String())
	if err != nil {
		return err
	}

	if address.Cmp(addressMin) < 0 {
		return errInvalidAddress
	}

	return nil
}

func (p *predeployParams) initChain() error {
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

func (p *predeployParams) updateGenesisConfig() error {
	if p.genesisConfig.Genesis.Alloc[p.address] != nil {
		return errAddressTaken
	}

	predeployAccount, err := predeployment.GenerateGenesisAccountFromFile(
		p.artifactsPath,
		p.constructorArgs,
		p.address,
		p.genesisConfig.Params.ChainID,
	)
	if err != nil {
		return err
	}

	p.genesisConfig.Genesis.Alloc[p.address] = predeployAccount

	return nil
}

func (p *predeployParams) overrideGenesisConfig() error {
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

func (p *predeployParams) getResult() command.CommandResult {
	return &GenesisPredeployResult{
		Address: p.address.String(),
	}
}
