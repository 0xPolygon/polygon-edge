package polybft

import (
	"math/big"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
)

const (
	// quorumSize is defined as 67% (math.Ceil(2 * float64(100) / 3))
	quorumSize = 67
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

	// votingPowerMap represents voting powers per validator address
	votingPowerMap map[types.Address]uint64

	// total voting power
	totalVotingPower int64

	// logger instance
	logger hclog.Logger
}

// NewValidatorSet creates a new validator set.
func NewValidatorSet(valz AccountSet, logger hclog.Logger) (*validatorSet, error) {
	valSet := &validatorSet{
		validators:       valz,
		votingPowerMap:   make(map[types.Address]uint64, valz.Len()),
		totalVotingPower: int64(0),
		logger:           logger.Named("validator_set"),
	}

	totalVotingPower := big.NewInt(0)
	for _, val := range valz {
		totalVotingPower = totalVotingPower.Add(totalVotingPower, val.VotingPower)
		// valSet.votingPowerMap[val.Address] = val.VotingPower.Int64()

		// // mind overflow
		// valSet.totalVotingPower = safeAddClip(valSet.totalVotingPower, val.VotingPower.Int64())
		// if valSet.totalVotingPower > maxTotalVotingPower {
		// 	return nil, fmt.Errorf(
		// 		"total voting power cannot be guarded to not exceed %v; got: %v",
		// 		maxTotalVotingPower,
		// 		valSet.totalVotingPower,
		// 	)
		// }
	}

	for _, val := range valz {
		valSet.votingPowerMap[val.Address] = val.getRelativeVotingPower(totalVotingPower)
	}

	return valSet, nil
}

// HasQuorum determines if there is quorum of enough signers reached,
// based on its voting power and quorum size
func (vs validatorSet) HasQuorum(signers map[types.Address]struct{}) bool {
	accVotingPower := uint64(0)

	for address := range signers {
		accVotingPower += vs.votingPowerMap[address]
	}

	vs.logger.Debug("HasQuorum",
		"signers", len(signers),
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
