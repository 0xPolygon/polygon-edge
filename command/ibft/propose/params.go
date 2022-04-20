package propose

import (
	"context"
	"errors"

	"github.com/dogechain-lab/jury/command"
	"github.com/dogechain-lab/jury/command/helper"
	ibftOp "github.com/dogechain-lab/jury/consensus/ibft/proto"
	"github.com/dogechain-lab/jury/types"
)

const (
	voteFlag    = "vote"
	addressFlag = "addr"
)

const (
	authVote = "auth"
	dropVote = "drop"
)

var (
	errInvalidVoteType      = errors.New("invalid vote type")
	errInvalidAddressFormat = errors.New("invalid address format")
)

var (
	params = &proposeParams{}
)

type proposeParams struct {
	addressRaw string

	vote    string
	address types.Address
}

func (p *proposeParams) getRequiredFlags() []string {
	return []string{
		voteFlag,
		addressFlag,
	}
}

func (p *proposeParams) validateFlags() error {
	if !isValidVoteType(p.vote) {
		return errInvalidVoteType
	}

	return nil
}

func (p *proposeParams) initRawParams() error {
	p.address = types.Address{}
	if err := p.address.UnmarshalText([]byte(p.addressRaw)); err != nil {
		return errInvalidAddressFormat
	}

	return nil
}

func isValidVoteType(vote string) bool {
	return vote == authVote || vote == dropVote
}

func (p *proposeParams) proposeCandidate(grpcAddress string) error {
	ibftClient, err := helper.GetIBFTOperatorClientConnection(grpcAddress)
	if err != nil {
		return err
	}

	if _, err := ibftClient.Propose(
		context.Background(),
		&ibftOp.Candidate{
			Address: p.address.String(),
			Auth:    p.vote == authVote,
		},
	); err != nil {
		return err
	}

	return nil
}

func (p *proposeParams) getResult() command.CommandResult {
	return &IBFTProposeResult{
		Address: p.address.String(),
		Vote:    p.vote,
	}
}
