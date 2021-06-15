package command

import (
	"context"
	"fmt"

	"github.com/0xPolygon/minimal/command/helper"
	"github.com/0xPolygon/minimal/consensus/ibft/proto"
	ibftOp "github.com/0xPolygon/minimal/consensus/ibft/proto"
	"github.com/0xPolygon/minimal/types"
)

// IbftPropose is the command to query the snapshot
type IbftPropose struct {
	Meta
}

// DefineFlags defines the command flags
func (p *IbftPropose) DefineFlags() {
	if p.flagMap == nil {
		// Flag map not initialized
		p.flagMap = make(map[string]helper.FlagDescriptor)
	}

	p.flagMap["addr"] = helper.FlagDescriptor{
		Description: "Address of the account to be voted for",
		Arguments: []string{
			"ETH_ADDRESS",
		},
		ArgumentsOptional: false,
		FlagOptional:      false,
	}

	p.flagMap["vote"] = helper.FlagDescriptor{
		Description: "Proposes a change to the validator set (add = true, remove = false). Default: true",
		Arguments: []string{
			"VOTE",
		},
		ArgumentsOptional: false,
		FlagOptional:      true,
	}
}

// GetHelperText returns a simple description of the command
func (p *IbftPropose) GetHelperText() string {
	return "Proposes a new candidate to be added or removed from the validator set"
}

func (p *IbftPropose) GetBaseCommand() string {
	return "ibft-propose"
}

// Help implements the cli.IbftPropose interface
func (p *IbftPropose) Help() string {
	p.Meta.DefineFlags()
	p.DefineFlags()

	return helper.GenerateHelp(p.Synopsis(), helper.GenerateUsage(p.GetBaseCommand(), p.flagMap), p.flagMap)
}

// Synopsis implements the cli.IbftPropose interface
func (p *IbftPropose) Synopsis() string {
	return p.GetHelperText()
}

// Run implements the cli.IbftPropose interface
func (p *IbftPropose) Run(args []string) int {
	flags := p.FlagSet(p.GetBaseCommand())

	var vote bool
	var ethAddress string

	flags.BoolVar(&vote, "vote", true, "")
	flags.StringVar(&ethAddress, "addr", "", "")

	if err := flags.Parse(args); err != nil {
		p.UI.Error(err.Error())
		return 1
	}

	if ethAddress == "" {
		p.UI.Error("Account address not specified")
		return 1
	}

	var addr types.Address
	if err := addr.UnmarshalText([]byte(ethAddress)); err != nil {
		p.UI.Error("Failed to decode address")
		return 1
	}

	conn, err := p.Conn()
	if err != nil {
		p.UI.Error(err.Error())
		return 1
	}

	clt := ibftOp.NewIbftOperatorClient(conn)
	req := &proto.Candidate{
		Address: addr.String(),
		Auth:    vote,
	}

	_, err = clt.Propose(context.Background(), req)
	if err != nil {
		p.UI.Error(err.Error())
		return 1
	}

	output := "\n[IBFT PROPOSE]\n"

	if vote {
		output += fmt.Sprintf("Successfully voted for the addition of address [%s] to the validator set", ethAddress)
	} else {
		output += fmt.Sprintf("Successfully voted for the removal of validator at address [%s] from the validator set", ethAddress)
	}

	return 0
}
