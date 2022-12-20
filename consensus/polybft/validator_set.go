package polybft

import (
	"fmt"
	"math"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
)

// ValidatorSet interface of the current validator set
type ValidatorSet interface {
	// Includes check if given address is among the current validator set
	Includes(address types.Address) bool

	// Len returns the size of the validator set
	Len() int

	// Accounts returns the list of the ValidatorMetadata
	Accounts() AccountSet

	// checks if submitted signers have reached quorum
	HasQuorum(signers map[types.Address]struct{}) bool
}

type validatorSet struct {
	// validators represents current list of validators
	validators AccountSet

	// votingPowerScalingFactor represents scaling
	votingPowerScalingFactor uint64

	// votingPowerMap represents voting powers per validator address
	votingPowerMap map[types.Address]int64

	// total voting power
	totalVotingPower int64

	// logger instance
	logger hclog.Logger
}

// NewValidatorSet creates a new validator set.
func NewValidatorSet(valz AccountSet, logger hclog.Logger, opts ...validatorSetOption) (*validatorSet, error) {
	valSet := &validatorSet{
		validators:       valz,
		votingPowerMap:   make(map[types.Address]int64, valz.Len()),
		totalVotingPower: int64(0),
		logger:           logger.Named("validator_set"),
	}

	for _, opt := range opts {
		opt(valSet)
	}

	if valSet.votingPowerScalingFactor == 0 {
		valSet.votingPowerScalingFactor = chain.WeiScalingFactor
	}

	for _, val := range valz {
		scaledVotingPower := val.getScaledVotingPower(valSet.votingPowerScalingFactor)
		valSet.votingPowerMap[val.Address] = scaledVotingPower

		// mind overflow
		valSet.totalVotingPower = safeAddClip(valSet.totalVotingPower, scaledVotingPower)
		if valSet.totalVotingPower > maxTotalVotingPower {
			return nil, fmt.Errorf(
				"total voting power cannot be guarded to not exceed %v; got: %v",
				maxTotalVotingPower,
				valSet.totalVotingPower,
			)
		}
	}

	return valSet, nil
}

// HasQuorum determines if there is quorum of enough signers reached,
// based on its voting power and quorum size
func (vs validatorSet) HasQuorum(signers map[types.Address]struct{}) bool {
	accVotingPower := int64(0)

	for address := range signers {
		value := vs.votingPowerMap[address] // will be 0 if signer does not exist
		accVotingPower = safeAddClip(accVotingPower, value)
	}

	quorumSize := vs.getQuorumSize()

	vs.logger.Debug("HasQuorum",
		"signers cnt", len(signers),
		"signers voting power", accVotingPower,
		"quorum", quorumSize,
		"hasQuorum", accVotingPower >= quorumSize)

	return accVotingPower >= quorumSize
}

func (vs validatorSet) Accounts() AccountSet {
	return vs.validators
}

func (vs validatorSet) Includes(address types.Address) bool {
	return vs.validators.ContainsAddress(address)
}

func (vs validatorSet) Len() int {
	return vs.validators.Len()
}

func (vs validatorSet) getQuorumSize() int64 {
	return int64(math.Ceil((2 * float64(vs.totalVotingPower)) / 3))
}

type validatorSetOption func(*validatorSet)

func withScalingFactor(scalingFactor uint64) validatorSetOption {
	return func(v *validatorSet) {
		v.votingPowerScalingFactor = scalingFactor
	}
}
