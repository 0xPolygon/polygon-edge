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
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/0xPolygon/polygon-edge/validators"
	"github.com/coinbase/kryptology/pkg/signatures/bls/bls_sig"
)

const (
	voteFlag         = "vote"
	addressFlag      = "addr"
	blsPublicKeyFlag = "bls-pubkey"
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
	rawIBFTValidatorType string
	addressRaw           string
	rawBLSPublicKey      string

	vote          string
	validatorType validators.ValidatorType
	address       types.Address
	blsPublicKey  []byte
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

	if err := p.validatorType.FromString(p.rawIBFTValidatorType); err != nil {
		return err
	}

	if err := p.initBLSPublicKey(); err != nil {
		return err
	}

	return nil
}

func (p *proposeParams) initBLSPublicKey() error {
	if p.validatorType == validators.BLSValidatorType {
		if p.rawBLSPublicKey == "" {
			return fmt.Errorf("BLS Public Key is required")
		}

		blsPubkeyBytes, err := hex.DecodeString(strings.TrimPrefix(p.rawBLSPublicKey, "0x"))
		if err != nil {
			return fmt.Errorf("failed to parse BLS Public Key: %w", err)
		}

		if err := validateBLSPublicKey(blsPubkeyBytes); err != nil {
			return err
		}

		p.blsPublicKey = blsPubkeyBytes
	} else {
		if p.rawBLSPublicKey != "" {
			return fmt.Errorf("BLS Public Key can't be specified")
		}
	}

	return nil
}

func isValidVoteType(vote string) bool {
	return vote == authVote || vote == dropVote
}

func validateBLSPublicKey(keyBytes []byte) error {
	pk := &bls_sig.PublicKey{}
	if err := pk.UnmarshalBinary(keyBytes); err != nil {
		return err
	}

	return nil
}

func (p *proposeParams) proposeCandidate(grpcAddress string) error {
	ibftClient, err := helper.GetIBFTOperatorClientConnection(grpcAddress)
	if err != nil {
		return err
	}

	if _, err := ibftClient.Propose(
		context.Background(),
		&ibftOp.Candidate{
			Data: p.GenerateCandidateBytes(),
			Auth: p.vote == authVote,
		},
	); err != nil {
		return err
	}

	return nil
}

func (p *proposeParams) GenerateCandidateBytes() []byte {
	switch p.validatorType {
	case validators.ECDSAValidatorType:
		val := &validators.ECDSAValidator{
			Address: p.address,
		}

		return val.Bytes()
	case validators.BLSValidatorType:
		val := &validators.BLSValidator{
			Address:      p.address,
			BLSPublicKey: p.blsPublicKey,
		}

		return val.Bytes()
	}

	return nil
}

func (p *proposeParams) getResult() command.CommandResult {
	return &IBFTProposeResult{
		Address: p.address.String(),
		Vote:    p.vote,
	}
}
