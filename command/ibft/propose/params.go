package propose

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/helper"
	ibftOp "github.com/0xPolygon/polygon-edge/consensus/ibft/proto"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/types"
)

const (
	voteFlag    = "vote"
	addressFlag = "addr"
	blsFlag     = "bls"
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
	addressRaw      string
	rawBLSPublicKey string

	vote         string
	address      types.Address
	blsPublicKey []byte
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
	if err := p.initAddress(); err != nil {
		return err
	}

	if err := p.initBLSPublicKey(); err != nil {
		return err
	}

	return nil
}

func (p *proposeParams) initAddress() error {
	p.address = types.Address{}
	if err := p.address.UnmarshalText([]byte(p.addressRaw)); err != nil {
		return errInvalidAddressFormat
	}

	return nil
}

func (p *proposeParams) initBLSPublicKey() error {
	if p.rawBLSPublicKey == "" {
		return nil
	}

	blsPubkeyBytes, err := hex.DecodeString(strings.TrimPrefix(p.rawBLSPublicKey, "0x"))
	if err != nil {
		return fmt.Errorf("failed to parse BLS Public Key: %w", err)
	}

	if _, err := crypto.UnmarshalBLSPublicKey(blsPubkeyBytes); err != nil {
		return err
	}

	p.blsPublicKey = blsPubkeyBytes

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
		p.getCandidate(),
	); err != nil {
		return err
	}

	return nil
}

func (p *proposeParams) getCandidate() *ibftOp.Candidate {
	res := &ibftOp.Candidate{
		Address: p.address.String(),
		Auth:    p.vote == authVote,
	}

	if p.blsPublicKey != nil {
		res.BlsPubkey = p.blsPublicKey
	}

	return res
}

func (p *proposeParams) getResult() command.CommandResult {
	return &IBFTProposeResult{
		Address: p.address.String(),
		Vote:    p.vote,
	}
}
