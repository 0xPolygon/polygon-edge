package polybft

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"math/big"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
)

const (
	// maxTotalVotingPower - the maximum allowed total voting power.
	// It needs to be sufficiently small to, in all cases:
	// 1. prevent clipping in incrementProposerPriority()
	// 2. let (diff+diffMax-1) not overflow in IncrementProposerPriority()
	// (Proof of 1 is tricky, left to the reader).
	// It could be higher, but this is sufficiently large for our purposes,
	// and leaves room for defensive purposes.
	maxTotalVotingPower = int64(math.MaxInt64) / 8

	// priorityWindowSizeFactor - is a constant that when multiplied with the
	// total voting power gives the maximum allowed distance between validator
	// priorities.
	priorityWindowSizeFactor = 2
)

var (
	// errInvalidTotalVotingPower is returned if the total voting power is zero
	errInvalidTotalVotingPower = errors.New(
		"invalid voting power configuration provided: total voting power must be greater than 0")
)

type ValidatorAccount struct {
	Metadata         *ValidatorMetadata
	ProposerPriority int64
}

// NewValidator returns a new validator with the given pubkey and voting power.
func NewValidator(metadata *ValidatorMetadata, priority int64) *ValidatorAccount {
	return &ValidatorAccount{
		Metadata:         metadata,
		ProposerPriority: priority,
	}
}

// CompareProposerPriority returns the one with higher proposer priority.
func (v *ValidatorAccount) CompareProposerPriority(other *ValidatorAccount) (*ValidatorAccount, error) {
	if v == nil {
		return other, nil
	}

	switch {
	case v.ProposerPriority > other.ProposerPriority:
		return v, nil
	case v.ProposerPriority < other.ProposerPriority:
		return other, nil
	default:
		result := bytes.Compare(v.Metadata.Address.Bytes(), other.Metadata.Address.Bytes())

		switch {
		case result < 0:
			return v, nil
		case result > 0:
			return other, nil
		default:
			return nil, fmt.Errorf("cannot compare identical validators")
		}
	}
}

// ValidatorSet interface of the current validator set
type ValidatorSet interface {
	// CalcProposer calculates next proposer based on the passed round
	CalcProposer(round uint64) (types.Address, error)

	// Includes checks if the passed address in included in the current validator set
	Includes(address types.Address) bool

	// Len returns the size of the validator set
	Len() int

	// Accounts returns the list of the ValidatorMetadata
	Accounts() AccountSet

	// IncrementProposerPriority increments priorities number of times
	IncrementProposerPriority(times uint64) error

	// checks if submitted signers have reached quorum
	HasQuorum(signers []types.Address) bool

	// checks if submitted signers have reached prepare quorum
	HasPrepareQuorum(signers []types.Address) bool
}

type validatorSet struct {
	// last proposer of a block
	last types.Address

	// validators represents current list of validators with their priority
	validators []*ValidatorAccount

	// proposer of a block
	proposer *ValidatorAccount

	// totalVotingPower denotes voting power of entire validator set
	totalVotingPower int64

	// quorum
	quorumSize uint64

	// votingPowerMap represents voting powers per validator address
	votingPowerMap map[types.Address]uint64

	// logger instance
	logger hclog.Logger
}

// NewValidatorSet creates a new validator set.
func NewValidatorSet(valz AccountSet, logger hclog.Logger) (*validatorSet, error) {
	var validators = make([]*ValidatorAccount, len(valz))
	for i, v := range valz {
		validators[i] = NewValidator(v, 0)
	}

	validatorSet := &validatorSet{
		validators: validators,
		logger:     logger.Named("validator_set"),
	}

	// populate voting power map
	validatorSet.populateVotingPower()

	// calculate quorum according to submitted voting powers
	err := validatorSet.calculateQuorum()
	if err != nil {
		return nil, err
	}

	err = validatorSet.updateWithChangeSet()
	if err != nil {
		return nil, fmt.Errorf("cannot update changeset: %w", err)
	}

	if len(valz) > 0 {
		err = validatorSet.IncrementProposerPriority(1)
		if err != nil {
			return nil, fmt.Errorf("cannot create validator set: %w", err)
		}
	}

	return validatorSet, nil
}

// populateVotingPower populates voting power map
// for each validator in the current validator set
func (v *validatorSet) populateVotingPower() {
	v.votingPowerMap = make(map[types.Address]uint64, len(v.validators))
	for _, validator := range v.validators {
		v.votingPowerMap[validator.Metadata.Address] = validator.Metadata.VotingPower
	}
}

// calculateQuorum calculates quorum size for given voting power map
func (v *validatorSet) calculateQuorum() error {
	totalVotingPower := uint64(0)
	for _, v := range v.votingPowerMap {
		totalVotingPower += v
	}

	if totalVotingPower == 0 {
		return errInvalidTotalVotingPower
	}

	// If there cannot be faulty nodes (less than 4 nodes in the network),
	// then quorum size is determined as total voting power (namely all the nodes must send vote).
	// Otherwise quorum size is calculated as 2/3 supermajority
	if v.calcMaxFaultyNodes() == 0 {
		v.quorumSize = totalVotingPower
	} else {
		v.quorumSize = uint64(math.Ceil(float64((2 * totalVotingPower) / 3)))
	}

	v.logger.Info("calculateQuorum", "quorum", v.quorumSize, "voting powers map", v.votingPowerMap)

	return nil
}

// IncrementProposerPriority increments ProposerPriority of each validator and
// updates the proposer.
func (v *validatorSet) IncrementProposerPriority(times uint64) error {
	if v.IsNilOrEmpty() {
		return fmt.Errorf("validator set cannot be nul or empty")
	}

	if times == 0 {
		return fmt.Errorf("cannot call IncrementProposerPriority with non-positive times")
	}

	// Cap the difference between priorities to be proportional to 2*totalPower by
	// re-normalizing priorities, i.e., rescale all priorities by multiplying with:
	//  2*totalVotingPower/(maxPriority - minPriority)

	vp, err := v.TotalVotingPower()

	if err != nil {
		return fmt.Errorf("cannot calculate total voting power")
	}

	diffMax := priorityWindowSizeFactor * vp

	err = v.rescalePriorities(diffMax)
	if err != nil {
		return fmt.Errorf("cannot rescale priorities: %w", err)
	}

	err = v.shiftByAvgProposerPriority()
	if err != nil {
		return fmt.Errorf("cannot shift avg priorities: %w", err)
	}

	var proposer *ValidatorAccount
	// Call IncrementProposerPriority(1) times times.
	for i := uint64(0); i < times; i++ {
		proposer, err = v.incrementProposerPriority()
		if err != nil {
			return fmt.Errorf("cannot increment proposer priority: %w", err)
		}
	}

	v.proposer = proposer

	return nil
}

func (v *validatorSet) incrementProposerPriority() (*ValidatorAccount, error) {
	for _, val := range v.validators {
		// Check for overflow for sum.
		newPrio := safeAddClip(val.ProposerPriority, int64(val.Metadata.VotingPower))
		val.ProposerPriority = newPrio
	}
	// Decrement the validator with most ProposerPriority.
	mostest, err := v.getValWithMostPriority()
	if err != nil {
		return nil, fmt.Errorf("cannot get validator with most priority: %w", err)
	}
	// Mind the underflow.
	vp, err := v.TotalVotingPower()
	if err != nil {
		return nil, fmt.Errorf("cannot get total voting power: %w", err)
	}

	mostest.ProposerPriority = safeSubClip(mostest.ProposerPriority, vp)

	return mostest, nil
}

// TotalVotingPower returns the sum of the voting powers of all validators.
// It recomputes the total voting power if required.
func (v *validatorSet) TotalVotingPower() (int64, error) {
	if v.totalVotingPower == 0 {
		err := v.updateTotalVotingPower()
		if err != nil {
			return 0, fmt.Errorf("cannot update total voting power: %w", err)
		}
	}

	return v.totalVotingPower, nil
}

func (v *validatorSet) updateWithChangeSet() error {
	err := v.updateTotalVotingPower()
	if err != nil {
		return fmt.Errorf("cannot update total voting power: %w", err)
	}
	// Scale and center.
	totalVotingPower, err := v.TotalVotingPower()
	if err != nil {
		return fmt.Errorf("cannot get total voting power: %w", err)
	}

	err = v.rescalePriorities(priorityWindowSizeFactor * totalVotingPower)
	if err != nil {
		return fmt.Errorf("cannot rescale priorities: %w", err)
	}

	err = v.shiftByAvgProposerPriority()
	if err != nil {
		return fmt.Errorf("cannot shift proposer priorities: %w", err)
	}

	return nil
}

func (v *validatorSet) shiftByAvgProposerPriority() error {
	avgProposerPriority, err := v.computeAvgProposerPriority()
	if err != nil {
		return fmt.Errorf("cannot compute proposer priority: %w", err)
	}

	for _, val := range v.validators {
		val.ProposerPriority = safeSubClip(val.ProposerPriority, avgProposerPriority)
	}

	return nil
}

func (v *validatorSet) getValWithMostPriority() (*ValidatorAccount, error) {
	var (
		res *ValidatorAccount
		err error
	)

	for _, val := range v.validators {
		res, err = res.CompareProposerPriority(val)
		if err != nil {
			return nil, fmt.Errorf("cannot compare proposer priority: %w", err)
		}
	}

	return res, nil
}

// Should not be called on an empty validator set.
func (v *validatorSet) computeAvgProposerPriority() (int64, error) {
	if v.IsNilOrEmpty() {
		return 0, fmt.Errorf("validator set cannot be nul or empty")
	}

	n := int64(len(v.validators))
	sum := big.NewInt(0)

	for _, val := range v.validators {
		sum.Add(sum, big.NewInt(val.ProposerPriority))
	}

	avg := sum.Div(sum, big.NewInt(n))
	if avg.IsInt64() {
		return avg.Int64(), nil
	}

	return 0, fmt.Errorf("cannot represent avg ProposerPriority as an int64 %v", avg)
}

// rescalePriorities rescales the priorities such that the distance between the
// maximum and minimum is smaller than `diffMax`.
func (v *validatorSet) rescalePriorities(diffMax int64) error {
	if v.IsNilOrEmpty() {
		return fmt.Errorf("validator set cannot be nul or empty")
	}
	// NOTE: This check is merely a sanity check which could be
	// removed if all tests would init. voting power appropriately;
	// i.e. diffMax should always be > 0
	if diffMax <= 0 {
		return fmt.Errorf("difference between priorities must be positive")
	}

	// Calculating ceil(diff/diffMax):
	// Re-normalization is performed by dividing by an integer for simplicity.
	// NOTE: This may make debugging priority issues easier as well.
	diff := computeMaxMinPriorityDiff(v)
	ratio := (diff + diffMax - 1) / diffMax

	if diff > diffMax {
		for _, val := range v.validators {
			val.ProposerPriority /= ratio
		}
	}

	return nil
}

// HasQuorum determines if there is quorum of enough signers reached,
// based on its voting power and quorum size
func (v *validatorSet) HasQuorum(signers []types.Address) bool {
	votingPower := v.calculateVotingPower(signers)
	v.logger.Debug("HasQuorum", "voting power", votingPower, "quorum", v.quorumSize,
		"hasQuorum", votingPower >= v.quorumSize)

	return votingPower >= v.quorumSize
}

// checks if submitted signers have reached prepare quorum
func (v *validatorSet) HasPrepareQuorum(signers []types.Address) bool {
	votingPower := v.calculateVotingPower(signers)
	v.logger.Debug("HasPrepareQuorum", "voting power", votingPower, "quorum", v.quorumSize,
		"hasQuorum", votingPower >= v.quorumSize-1)

	return votingPower >= v.quorumSize-1
}

// calcMaxFaultyNodes calculates maximum faulty nodes in order to have Byzantine fault tollerant properties
func (v validatorSet) calcMaxFaultyNodes() uint64 {
	return uint64((v.Len() - 1) / 3)
}

// calculateVotingPower calculates voting power for provided validator ids
func (v validatorSet) calculateVotingPower(signers []types.Address) uint64 {
	accumulatedVotingPower := uint64(0)
	for _, address := range signers {
		accumulatedVotingPower += v.votingPowerMap[address]
	}

	return accumulatedVotingPower
}

// IsNilOrEmpty returns true if validator set is nil or empty.
func (v *validatorSet) IsNilOrEmpty() bool {
	return v == nil || len(v.validators) == 0
}

// updateTotalVotingPower forces recalculation of the set's total voting power.
func (v *validatorSet) updateTotalVotingPower() error {
	sum := int64(0)
	for _, val := range v.validators {
		// mind overflow
		sum = safeAddClip(sum, int64(val.Metadata.VotingPower))
		if sum > maxTotalVotingPower {
			return fmt.Errorf(
				"total voting power cannot be guarded to not exceed %v; got: %v",
				maxTotalVotingPower,
				sum,
			)
		}
	}

	v.totalVotingPower = sum

	return nil
}

func (v *validatorSet) Accounts() AccountSet {
	var accountSet = make([]*ValidatorMetadata, len(v.validators))
	for i, validator := range v.validators {
		accountSet[i] = validator.Metadata
	}

	return accountSet
}

func (v *validatorSet) CalcProposer(round uint64) (types.Address, error) {
	vc := v.Copy()
	err := vc.IncrementProposerPriority(round + 1) // if round = 0 then we need one iteration

	if err != nil {
		return types.ZeroAddress, fmt.Errorf("cannot increment proposer priority: %w", err)
	}

	proposer, err := vc.getProposer()
	if err != nil {
		return types.ZeroAddress, fmt.Errorf("cannot get proposer: %w", err)
	}

	return proposer.Metadata.Address, nil
}

func (v *validatorSet) Includes(address types.Address) bool {
	for _, validator := range v.validators {
		if validator.Metadata.Address == address {
			return true
		}
	}

	return false
}

func (v *validatorSet) Len() int {
	return len(v.validators)
}

// getProposer returns the current proposer
func (v *validatorSet) getProposer() (*ValidatorAccount, error) {
	if v.IsNilOrEmpty() {
		return nil, fmt.Errorf("validators cannot be nil or empty")
	}

	if v.proposer == nil {
		proposer, err := v.findProposer()

		if err != nil {
			return nil, fmt.Errorf("cannot find proposer: %w", err)
		}

		v.proposer = proposer
	}

	return NewValidator(v.proposer.Metadata, v.proposer.ProposerPriority), nil
}

func (v *validatorSet) findProposer() (*ValidatorAccount, error) {
	var (
		proposer *ValidatorAccount
		err      error
	)

	for _, val := range v.validators {
		if proposer == nil || !bytes.Equal(val.Metadata.Address.Bytes(), proposer.Metadata.Address.Bytes()) {
			proposer, err = proposer.CompareProposerPriority(val)
			if err != nil {
				return nil, fmt.Errorf("cannot compare proposer priority: %w", err)
			}
		}
	}

	return proposer, nil
}

// Compute the difference between the max and min ProposerPriority of that set.
func computeMaxMinPriorityDiff(v *validatorSet) int64 {
	max := int64(math.MinInt64)
	min := int64(math.MaxInt64)

	for _, v := range v.validators {
		if v.ProposerPriority < min {
			min = v.ProposerPriority
		}

		if v.ProposerPriority > max {
			max = v.ProposerPriority
		}
	}

	diff := max - min

	if diff < 0 {
		return -1 * diff
	}

	return diff
}

// Copy each validator into a new ValidatorSet.
func (v *validatorSet) Copy() *validatorSet {
	return &validatorSet{
		validators:       validatorListCopy(v.validators),
		proposer:         v.proposer,
		totalVotingPower: v.totalVotingPower,
	}
}

// validatorListCopy makes a copy of the validator list.
func validatorListCopy(valList []*ValidatorAccount) []*ValidatorAccount {
	valCopy := make([]*ValidatorAccount, len(valList))
	for i, val := range valList {
		valCopy[i] = NewValidator(val.Metadata, val.ProposerPriority)
	}

	return valCopy
}
