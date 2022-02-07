package ibft

import (
	"errors"
	"fmt"
	"github.com/0xPolygon/polygon-edge/chain"
	"os"

	"github.com/0xPolygon/polygon-edge/command/helper"
)

// IbftContractRestriction is the command to query the snapshot
type IbftContractRestriction struct {
	helper.Base
	Formatter *helper.FormatterFlag
	GRPC      *helper.GRPCFlag
}

func (p *IbftContractRestriction) DefineFlags() {
	p.Base.DefineFlags(p.Formatter, p.GRPC)

	p.FlagMap["accounts"] = helper.FlagDescriptor{
		Description: "Accounts that whill go into whitelist",
		Arguments: []string{
			"WHITE_LIST_ACCOUNT",
		},
		ArgumentsOptional: false,
		FlagOptional:      true,
	}
}

// GetHelperText returns a simple description of the command
func (p *IbftContractRestriction) GetHelperText() string {
	return "Sets a list of accounts to go into whitelist"
}

func (p *IbftContractRestriction) GetBaseCommand() string {
	return "ibft contract_whitelist"
}

// Help implements the cli.IbftSnapshot interface
func (p *IbftContractRestriction) Help() string {
	p.DefineFlags()

	return helper.GenerateHelp(p.Synopsis(), helper.GenerateUsage(p.GetBaseCommand(), p.FlagMap), p.FlagMap)
}

// Synopsis implements the cli.IbftSnapshot interface
func (p *IbftContractRestriction) Synopsis() string {
	return p.GetHelperText()
}

func (p *IbftContractRestriction) Run(args []string) int {

	flags := p.Base.NewFlagSet(p.GetBaseCommand(), p.Formatter, p.GRPC)

	// query a specific snapshot
	var account, genesisPath string

	flags.StringVar(&account, "addAccount", "", "")
	flags.StringVar(&genesisPath, "chain", "genesis.json", "")
	if err := flags.Parse(args); err != nil {
		p.Formatter.OutputError(err)

		return 1
	}
	if account == "" {
		p.Formatter.OutputError(errors.New("Adddress not specified"))

		return 1
	}

	cc, err := chain.Import(genesisPath)
	if err != nil {
		p.Formatter.OutputError(fmt.Errorf("failed to load chain config from %s: %w", genesisPath, err))

		return 1
	}

	if err := appendIBFTForks(cc, account); err != nil {
		p.Formatter.OutputError(err)

		return 1
	}

	// Remove current genesis
	if err := os.Remove("genesis.json"); err != nil {
		p.Formatter.OutputError(err)

		return 1
	}

	// Save new genesis
	if err = helper.WriteGenesisToDisk(cc, "genesis.json"); err != nil {
		p.UI.Error(err.Error())

		return 1
	}

	return 0
}

func appendIBFTForks(cc *chain.Chain, account string) error {
	cc.Params.Features.ContractDeployment.WhiteList = append(cc.Params.Features.ContractDeployment.WhiteList, account)

	return nil
}
