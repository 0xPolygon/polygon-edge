package polybft

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"math/big"
	"sync"

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

// ProposerCalculator interface of the current validator set
type ProposerCalculator interface {
	// CalcProposer calculates next proposer based on the passed round
	CalcProposer(round uint64) (types.Address, error)

	// IncrementProposerPriority increments priorities number of times
	IncrementProposerPriority(times uint64) error

	// GetLatestProposer returns latest calculated proposer
	GetLatestProposer(round uint64) (types.Address, bool)
}

type validatorCalcMetadata struct {
	Metadata         *ValidatorMetadata
	ProposerPriority int64
}

func (a validatorCalcMetadata) IsBetter(b *validatorCalcMetadata) bool {
	if b == nil || a.ProposerPriority > b.ProposerPriority {
		return true
	} else if a.ProposerPriority == b.ProposerPriority {
		return bytes.Compare(a.Metadata.Address.Bytes(), b.Metadata.Address.Bytes()) <= 0
	} else {
		return false
	}
}

// newValidatorCalcMetadata returns a new validator with the given pubkey and voting power.
func newValidatorCalcMetadata(metadata *ValidatorMetadata, priority int64) *validatorCalcMetadata {
	return &validatorCalcMetadata{
		Metadata:         metadata,
		ProposerPriority: priority,
	}
}

type proposerCalculator struct {
	// validators represents current list of validators with their priority
	validators []*validatorCalcMetadata

	// proposer of a block
	proposer *validatorCalcMetadata

	// current proposer is calculated in this round - optimization
	round uint64

	// total voting power
	totalVotingPower int64

	// rw mutex
	lock *sync.RWMutex

	// logger instance
	logger hclog.Logger
}

// NewProposerCalculator creates a new proposer calculator object.
func NewProposerCalculator(valz AccountSet, totalVotingPower int64, logger hclog.Logger) (*proposerCalculator, error) {
	var validators = make([]*validatorCalcMetadata, len(valz))
	for i, v := range valz {
		validators[i] = newValidatorCalcMetadata(v, 0)
	}

	proposerCalc := &proposerCalculator{
		totalVotingPower: totalVotingPower,
		validators:       validators,
		lock:             &sync.RWMutex{},
		round:            0,
		logger:           logger.Named("validator_set"),
	}

	if err := proposerCalc.updateWithChangeSet(); err != nil {
		return nil, fmt.Errorf("cannot update changeset: %w", err)
	}

	return proposerCalc, nil
}

// GetLatestProposer returns address of the latest calculated proposer or false if there is no proposer
func (pc proposerCalculator) GetLatestProposer(round uint64) (types.Address, bool) {
	pc.lock.RLock()
	defer pc.lock.RUnlock()

	// round must be same as saved one and proposer must exist
	if pc.proposer == nil || pc.round != round {
		return types.ZeroAddress, false
	}

	return pc.proposer.Metadata.Address, true
}

// CalcProposer returns proposer address or error
func (pc *proposerCalculator) CalcProposer(round uint64) (types.Address, error) {
	// optimization -> return current proposer if already calculated for this round
	pc.lock.RLock()
	currentProposer := pc.proposer
	isSameRound := round == pc.round && currentProposer != nil
	pc.lock.RUnlock()

	if isSameRound {
		return currentProposer.Metadata.Address, nil
	}

	clone := pc.Copy()

	// if round = 0 then we need one iteration
	if err := clone.IncrementProposerPriority(round + 1); err != nil {
		return types.ZeroAddress, fmt.Errorf("cannot increment proposer priority: %w", err)
	}

	var err error

	proposer := clone.proposer // no need for lock here because clone is temporary copy
	if proposer == nil {
		// try to retrieve validator with highest priority
		if proposer, err = clone.getValWithMostPriority(); err != nil {
			return types.ZeroAddress, err
		}
	}

	// keep proposer in the original validator set
	pc.lock.Lock()
	pc.proposer = proposer
	pc.round = round
	pc.lock.Unlock()

	return proposer.Metadata.Address, nil
}

// IncrementProposerPriority increments ProposerPriority of each validator and
// updates the proposer.
func (pc *proposerCalculator) IncrementProposerPriority(times uint64) error {
	if pc.isNilOrEmpty() {
		return fmt.Errorf("validator set cannot be nul or empty")
	}

	if times == 0 {
		return fmt.Errorf("cannot call IncrementProposerPriority with non-positive times")
	}

	if err := pc.rescalePriorities(); err != nil {
		return fmt.Errorf("cannot rescale priorities: %w", err)
	}

	if err := pc.shiftByAvgProposerPriority(); err != nil {
		return fmt.Errorf("cannot shift avg priorities: %w", err)
	}

	var (
		proposer *validatorCalcMetadata
		err      error
	)

	for i := uint64(0); i < times; i++ {
		if proposer, err = pc.incrementProposerPriority(); err != nil {
			return fmt.Errorf("cannot increment proposer priority: %w", err)
		}
	}

	pc.lock.Lock()
	pc.proposer = proposer
	pc.lock.Unlock()

	return nil
}

func (pc *proposerCalculator) incrementProposerPriority() (*validatorCalcMetadata, error) {
	for _, val := range pc.validators {
		// Check for overflow for sum.
		newPrio := safeAddClip(val.ProposerPriority, int64(val.Metadata.VotingPower))
		val.ProposerPriority = newPrio
	}
	// Decrement the validator with most ProposerPriority.
	mostest, err := pc.getValWithMostPriority()
	if err != nil {
		return nil, fmt.Errorf("cannot get validator with most priority: %w", err)
	}

	mostest.ProposerPriority = safeSubClip(mostest.ProposerPriority, pc.totalVotingPower)

	return mostest, nil
}

func (pc *proposerCalculator) updateWithChangeSet() error {
	if err := pc.rescalePriorities(); err != nil {
		return fmt.Errorf("cannot rescale priorities: %w", err)
	}

	if err := pc.shiftByAvgProposerPriority(); err != nil {
		return fmt.Errorf("cannot shift proposer priorities: %w", err)
	}

	return nil
}

func (pc *proposerCalculator) shiftByAvgProposerPriority() error {
	avgProposerPriority, err := pc.computeAvgProposerPriority()
	if err != nil {
		return fmt.Errorf("cannot compute proposer priority: %w", err)
	}

	for _, val := range pc.validators {
		val.ProposerPriority = safeSubClip(val.ProposerPriority, avgProposerPriority)
	}

	return nil
}

func (pc *proposerCalculator) getValWithMostPriority() (result *validatorCalcMetadata, err error) {
	if pc.isNilOrEmpty() {
		return nil, fmt.Errorf("validators cannot be nil or empty")
	}

	for _, curr := range pc.validators {
		// pick curr as result if it has greater priority
		// or if it has same priority but "smaller" address
		if curr.IsBetter(result) {
			result = curr
		}
	}

	return result, nil
}

func (pc *proposerCalculator) computeAvgProposerPriority() (int64, error) {
	if pc.isNilOrEmpty() {
		return 0, fmt.Errorf("validator set cannot be nul or empty")
	}

	n := int64(len(pc.validators))
	sum := big.NewInt(0)

	for _, val := range pc.validators {
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
func (pc *proposerCalculator) rescalePriorities() error {
	if pc.isNilOrEmpty() {
		return fmt.Errorf("validator set cannot be nul or empty")
	}

	// Cap the difference between priorities to be proportional to 2*totalPower by
	// re-normalizing priorities, i.e., rescale all priorities by multiplying with:
	// 2*totalVotingPower/(maxPriority - minPriority)
	diffMax := priorityWindowSizeFactor * pc.totalVotingPower

	// Calculating ceil(diff/diffMax):
	// Re-normalization is performed by dividing by an integer for simplicity.
	// NOTE: This may make debugging priority issues easier as well.
	diff := computeMaxMinPriorityDiff(pc.validators)
	ratio := (diff + diffMax - 1) / diffMax

	if diff > diffMax {
		for _, val := range pc.validators {
			val.ProposerPriority /= ratio
		}
	}

	return nil
}

// isNilOrEmpty returns true if validator set is nil or empty.
func (pc proposerCalculator) isNilOrEmpty() bool {
	return len(pc.validators) == 0
}

// Copy each validator into a new ValidatorSet.
func (pc proposerCalculator) Copy() *proposerCalculator {
	pc.lock.RLock()
	defer pc.lock.RUnlock()

	proposer := pc.proposer

	valCopy := make([]*validatorCalcMetadata, len(pc.validators))
	for i, val := range pc.validators {
		valCopy[i] = newValidatorCalcMetadata(val.Metadata, val.ProposerPriority)
	}

	return &proposerCalculator{
		validators:       valCopy,
		proposer:         proposer,
		lock:             &sync.RWMutex{},
		totalVotingPower: pc.totalVotingPower,
		round:            pc.round,
		logger:           pc.logger,
	}
}

// computeMaxMinPriorityDiff computes the difference between the max and min ProposerPriority of that set.
func computeMaxMinPriorityDiff(validators []*validatorCalcMetadata) int64 {
	max := int64(math.MinInt64)
	min := int64(math.MaxInt64)

	for _, v := range validators {
		if v.ProposerPriority < min {
			min = v.ProposerPriority
		}

		if v.ProposerPriority > max {
			max = v.ProposerPriority
		}
	}

	diff := max - min

	if diff < 0 {
		return -diff
	}

	return diff
}
