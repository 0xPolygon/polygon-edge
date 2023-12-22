package predeploy

import (
	"errors"
	"fmt"
	"os"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi/artifact"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/helper/predeployment"
	"github.com/0xPolygon/polygon-edge/types"
)

const (
	chainFlag               = "chain"
	predeployAddressFlag    = "predeploy-address"
	artifactsNameFlag       = "artifacts-name"
	artifactsPathFlag       = "artifacts-path"
	constructorArgsPathFlag = "constructor-args"
	deployerAddrFlag        = "deployer-address"
)

var (
	errInvalidPredeployAddress = errors.New("invalid predeploy address provided")
	errAddressTaken            = errors.New("the provided predeploy address is taken")
	errInvalidAddress          = fmt.Errorf(
		"the provided predeploy address must be >= %s", predeployAddressMin.String(),
	)

	predeployAddressMin = types.StringToAddress("01100")
	params              = &predeployParams{}
)

type predeployParams struct {
	addressRaw      string
	genesisPath     string
	deployerAddrRaw string

	address       types.Address
	deployerAddr  types.Address
	artifactsName string
	artifactsPath string

	constructorArgs []string

	genesisConfig    *chain.Chain
	contractArtifact *artifact.Artifact
}

func (p *predeployParams) getRequiredFlags() []string {
	return []string{
		predeployAddressFlag,
		artifactsNameFlag,
	}
}

func (p *predeployParams) initRawParams() (err error) {
	if err := p.initPredeployAddress(); err != nil {
		return err
	}

	if err := p.initDeployerAddress(); err != nil {
		return err
	}

	if err := p.verifyMinAddress(); err != nil {
		return err
	}

	if err := p.initChain(); err != nil {
		return err
	}

	if p.contractArtifact, err = contractsapi.GetArtifactFromArtifactName(p.artifactsName); err != nil {
		return err
	}

	return nil
}

func (p *predeployParams) initPredeployAddress() error {
	if p.addressRaw == "" {
		return errInvalidPredeployAddress
	}

	p.address = types.StringToAddress(p.addressRaw)

	return nil
}

func (p *predeployParams) initDeployerAddress() error {
	p.deployerAddr = types.StringToAddress(p.deployerAddrRaw)

	return nil
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
		p.contractArtifact,
		p.constructorArgs,
		p.address,
		p.genesisConfig.Params.ChainID,
		p.deployerAddr,
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
