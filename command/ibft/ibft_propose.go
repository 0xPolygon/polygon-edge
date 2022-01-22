package ibft

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/0xPolygon/polygon-edge/command/helper"
	ibftOp "github.com/0xPolygon/polygon-edge/consensus/ibft/proto"
	"github.com/0xPolygon/polygon-edge/types"
)

// IbftPropose is the command to propose a candidate and
// cast a vote for addition / removal from the validator set
type IbftPropose struct {
	helper.Base
	Formatter *helper.FormatterFlag
	GRPC      *helper.GRPCFlag
}

// DefineFlags defines the command flags
func (p *IbftPropose) DefineFlags() {
	p.Base.DefineFlags(p.Formatter, p.GRPC)

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
	flags := p.Base.NewFlagSet(p.GetBaseCommand(), p.Formatter, p.GRPC)

	var (
		vote       string
		ethAddress string
	)

	flags.StringVar(&vote, "vote", "", "")
	flags.StringVar(&ethAddress, "addr", "", "")

	if err := flags.Parse(args); err != nil {
		p.Formatter.OutputError(err)

		return 1
	}

	if vote == "" {
		p.Formatter.OutputError(errors.New("vote value not specified"))

		return 1
	}

	if vote != positive && vote != negative {
		p.Formatter.OutputError(fmt.Errorf("invalid vote value (should be '%s' or '%s')", positive, negative))

		return 1
	}

	if ethAddress == "" {
		p.Formatter.OutputError(errors.New("account address not specified"))

		return 1
	}

	var addr types.Address
	if err := addr.UnmarshalText([]byte(ethAddress)); err != nil {
		p.Formatter.OutputError(errors.New("failed to decode address"))

		return 1
	}

	conn, err := p.GRPC.Conn()
	if err != nil {
		p.Formatter.OutputError(err)

		return 1
	}

	clt := ibftOp.NewIbftOperatorClient(conn)
	req := &ibftOp.Candidate{
		Address: addr.String(),
		Auth:    vote == positive,
	}

	_, err = clt.Propose(context.Background(), req)
	if err != nil {
		p.Formatter.OutputError(err)

		return 1
	}

	res := &IBFTProposeResult{
		Address: addr.String(),
		Vote:    vote,
	}
	p.Formatter.OutputResult(res)

	return 0
}

type IBFTProposeResult struct {
	Address string `json:"-"`
	Vote    string `json:"-"`
}

func (r *IBFTProposeResult) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`{"message": "%s"}`, r.Message())), nil
}

func (r *IBFTProposeResult) Output() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[IBFT PROPOSE]\n")
	buffer.WriteString(r.Message())
	buffer.WriteString("\n")

	return buffer.String()
}

func (r *IBFTProposeResult) Message() string {
	if r.Vote == positive {
		return fmt.Sprintf("Successfully voted for the addition of address [%s] to the validator set", r.Address)
	} else {
		return fmt.Sprintf(
			"Successfully voted for the removal of validator at address [%s] from the validator set",
			r.Address,
		)
	}
}
