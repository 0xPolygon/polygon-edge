package ibft

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
	helper.Meta
}

// DefineFlags defines the command flags
func (p *IbftPropose) DefineFlags() {
	if p.FlagMap == nil {
		// Flag map not initialized
		p.FlagMap = make(map[string]helper.FlagDescriptor)
	}

	p.FlagMap["addr"] = helper.FlagDescriptor{
		Description: "Address of the account to be voted for",
		Arguments: []string{
			"ETH_ADDRESS",
		},
		ArgumentsOptional: false,
		FlagOptional:      false,
	}

	p.FlagMap["vote"] = helper.FlagDescriptor{
		Description: fmt.Sprintf(
			"Proposes a change to the validator set. Possible values: [%s, %s]", positive, negative,
		),
		Arguments: []string{
			"VOTE",
		},
		ArgumentsOptional: false,
		FlagOptional:      false,
	}
}

// GetHelperText returns a simple description of the command
func (p *IbftPropose) GetHelperText() string {
	return "Proposes a new candidate to be added or removed from the validator set"
}

func (p *IbftPropose) GetBaseCommand() string {
	return "ibft propose"
}

// Help implements the cli.IbftPropose interface
func (p *IbftPropose) Help() string {
	p.Meta.DefineFlags()
	p.DefineFlags()

	return helper.GenerateHelp(p.Synopsis(), helper.GenerateUsage(p.GetBaseCommand(), p.FlagMap), p.FlagMap)
}

// Synopsis implements the cli.IbftPropose interface
func (p *IbftPropose) Synopsis() string {
	return p.GetHelperText()
}

// Extracting it out so we have it in a variable
var (
	positive = "auth"
	negative = "drop"
)

// Run implements the cli.IbftPropose interface
func (p *IbftPropose) Run(args []string) int {
	flags := p.FlagSet(p.GetBaseCommand())

	var vote string
	var ethAddress string

	flags.StringVar(&vote, "vote", "", "")
	flags.StringVar(&ethAddress, "addr", "", "")

	if err := flags.Parse(args); err != nil {
		p.UI.Error(err.Error())
		return 1
	}

	if vote == "" {
		p.UI.Error("Vote value not specified")
		return 1
	}

	if vote != positive && vote != negative {
		p.UI.Error(fmt.Sprintf("Invalid vote value (should be '%s' or '%s')", positive, negative))
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
		Auth:    vote == positive,
	}

	_, err = clt.Propose(context.Background(), req)
	if err != nil {
		p.UI.Error(err.Error())
		return 1
	}

	output := "\n[IBFT PROPOSE]\n"

	if vote == positive {
		output += fmt.Sprintf("Successfully voted for the addition of address [%s] to the validator set\n", ethAddress)
	} else {
		output += fmt.Sprintf("Successfully voted for the removal of validator at address [%s] from the validator set\n", ethAddress)
	}

	p.UI.Info(output)

	return 0
}
