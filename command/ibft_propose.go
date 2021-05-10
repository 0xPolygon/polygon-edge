package command

import (
	"context"

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
		p.flagMap = make(map[string]FlagDescriptor)
	}

	if len(p.flagMap) > 0 {
		// No need to redefine the flags again
		return
	}

	p.flagMap["add"] = FlagDescriptor{
		description: "Proposes a new validator to be added to the validator set",
		arguments: []string{
			"ETH_ADDRESS",
		},
		argumentsOptional: false,
	}

	p.flagMap["del"] = FlagDescriptor{
		description: "Proposes a new validator to be removed from the validator set",
		arguments: []string{
			"ETH_ADDRESS",
		},
		argumentsOptional: false,
	}
}

// GetHelperText returns a simple description of the command
func (p *IbftPropose) GetHelperText() string {
	return "Proposes a new candidate to be added to the snapshot list"
}

// Help implements the cli.IbftPropose interface
func (p *IbftPropose) Help() string {
	p.DefineFlags()
	usage := "ibft propose [--add ETH_ADDRESS]\n\t"
	usage += "ibft propose [--del ETH_ADDRESS]"

	return p.GenerateHelp(p.Synopsis(), usage)
}

// Synopsis implements the cli.IbftPropose interface
func (p *IbftPropose) Synopsis() string {
	return p.GetHelperText()
}

// Run implements the cli.IbftPropose interface
func (p *IbftPropose) Run(args []string) int {
	flags := p.FlagSet("ibft propose")

	var add, del bool
	flags.BoolVar(&add, "add", false, "add")
	flags.BoolVar(&del, "del", false, "del")

	if err := flags.Parse(args); err != nil {
		p.UI.Error(err.Error())
		return 1
	}

	if add == del {
		// either (add=true, del=true) or (add=false, del=false)
		p.UI.Error("Either add or del needs to be set")
		return 1
	}

	args = flags.Args()
	if len(args) != 1 {
		p.UI.Error("only 1 argument expected")
		return 1
	}

	var addr types.Address
	if err := addr.UnmarshalText([]byte(args[0])); err != nil {
		p.UI.Error("failed to decode addr")
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
		Auth:    add,
	}
	_, err = clt.Propose(context.Background(), req)
	if err != nil {
		p.UI.Error(err.Error())
		return 1
	}
	return 0
}
