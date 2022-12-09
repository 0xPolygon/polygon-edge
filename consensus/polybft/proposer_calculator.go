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

type ProposerCalculatorValidator struct {
	Metadata         *ValidatorMetadata
	ProposerPriority int64
}

type ProposerCalculatorSnapshot struct {
	Height     uint64
	Validators []*ProposerCalculatorValidator
}

func NewProposerCalculatorSnapshot(height uint64, validators []*ValidatorMetadata) *ProposerCalculatorSnapshot {
	validatorsSnap := make([]*ProposerCalculatorValidator, len(validators))

	for i, x := range validators {
		validatorsSnap[i] = &ProposerCalculatorValidator{Metadata: x, ProposerPriority: int64(0)}
	}

	return &ProposerCalculatorSnapshot{Height: height, Validators: validatorsSnap}
}

func (pcs ProposerCalculatorSnapshot) GetTotalVotingPower() int64 {
	totalVotingPower := int64(0)

	for _, v := range pcs.Validators {
		totalVotingPower = safeAddClip(totalVotingPower, int64(v.Metadata.VotingPower))
	}

	return totalVotingPower
}

func (pcs *ProposerCalculatorSnapshot) Copy() *ProposerCalculatorSnapshot {
	valCopy := make([]*ProposerCalculatorValidator, len(pcs.Validators))
	for i, val := range pcs.Validators {
		valCopy[i] = &ProposerCalculatorValidator{Metadata: val.Metadata, ProposerPriority: val.ProposerPriority}
	}

	return &ProposerCalculatorSnapshot{
		Validators: valCopy,
		Height:     pcs.Height,
	}
}

// ProposerCalculator interface of the current validator set
type ProposerCalculator interface {
	// CalcProposer calculates next proposer based on the passed round
	CalcProposer(round, height uint64) (types.Address, error)

	// Update updates snapshot, re-center priorities if `len(newValidatorSet)>0`
	// and finally increments priorities `round + 1` number of times
	Update(round, height uint64, newValidatorSet AccountSet) error

	// GetLatestProposer returns latest calculated proposer
	GetLatestProposer(round, height uint64) (types.Address, bool)

	// Clone clones existing proposer calculator and also returns new snapshot
	Clone() (ProposerCalculator, *ProposerCalculatorSnapshot)
}

func isBetterProposer(a, b *ProposerCalculatorValidator) bool {
	if b == nil || a.ProposerPriority > b.ProposerPriority {
		return true
	} else if a.ProposerPriority == b.ProposerPriority {
		return bytes.Compare(a.Metadata.Address.Bytes(), b.Metadata.Address.Bytes()) <= 0
	} else {
		return false
	}
}

type proposerCalculator struct {
	// snapshot snapshot
	snapshot *ProposerCalculatorSnapshot

	// total voting power
	totalVotingPower int64

	// proposer calculator validator
	proposer *ProposerCalculatorValidator

	// rw mutex
	lock *sync.RWMutex

	// current round
	round uint64

	// logger instance
	logger hclog.Logger
}

// NewProposerCalculator creates a new proposer calculator object.
func NewProposerCalculator(snapshot *ProposerCalculatorSnapshot, logger hclog.Logger) (ProposerCalculator, error) {
	proposerCalc := &proposerCalculator{
		totalVotingPower: snapshot.GetTotalVotingPower(),
		lock:             &sync.RWMutex{},
		snapshot:         snapshot,
		round:            0,
		logger:           logger.Named("proposer_calculator"),
	}

	if err := proposerCalc.updateWithChangeSet(); err != nil {
		return nil, fmt.Errorf("cannot update changeset: %w", err)
	}

	return proposerCalc, nil
}

// GetLatestProposer returns address of the latest calculated proposer or false if there is no proposer
func (pc proposerCalculator) GetLatestProposer(round, height uint64) (types.Address, bool) {
	pc.lock.RLock()
	defer pc.lock.RUnlock()

	// round must be same as saved one and proposer must exist
	if pc.proposer == nil || pc.round != round || pc.snapshot.Height != height {
		pc.logger.Info("Get Latest proposer not found", "height", height, "round", round, "address",
			"curr height", pc.snapshot.Height, "curr round", pc.round)

		return types.ZeroAddress, false
	}

	pc.logger.Info("Get Latest proposer",
		"height", height, "round", round, "address", "address", pc.proposer.Metadata.Address)

	return pc.proposer.Metadata.Address, true
}

// CalcProposer returns proposer address or error
func (pc *proposerCalculator) CalcProposer(round, height uint64) (types.Address, error) {
	// optimization -> return current proposer if already calculated for this round
	pc.lock.RLock()
	currentProposer, currentHeight := pc.proposer, pc.snapshot.Height
	isSameRound := round == pc.round && currentProposer != nil
	pc.lock.RUnlock()

	if currentHeight != height {
		return types.ZeroAddress, fmt.Errorf("calc proposer wrong height = %d, current height = %d", height, currentHeight)
	}

	if isSameRound {
		return currentProposer.Metadata.Address, nil
	}

	clone := pc.Copy()

	// if round = 0 then we need one iteration
	if err := clone.incrementProposerPriorityNTimes(round + 1); err != nil {
		return types.ZeroAddress, err
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

	pc.logger.Info("New proposer calculated", "height", height, "round", round, "address", proposer.Metadata.Address)

	return proposer.Metadata.Address, nil
}

// Update TODO: this method is not thread safe - it is used only on temp reference from one routine
func (pc *proposerCalculator) Update(round, height uint64, newValidatorSet AccountSet) error {
	if pc.snapshot.Height+1 != height {
		return fmt.Errorf("proposer calculator update wrong height %d. current height = %d",
			height, pc.snapshot.Height)
	}

	// if round = 0 then we need one iteration
	if err := pc.incrementProposerPriorityNTimes(round + 1); err != nil {
		return err
	}

	// update to new validator set and center if needed
	pc.updateValidators(newValidatorSet)

	pc.snapshot.Height = height
	pc.round = 0
	pc.proposer = nil

	return nil
}

// Clone clones existing proposer calculator and also returns new snapshot
func (pc *proposerCalculator) Clone() (ProposerCalculator, *ProposerCalculatorSnapshot) {
	pc.lock.RLock()
	defer pc.lock.RUnlock()

	snapshot := pc.snapshot.Copy()
	proposerCalc := &proposerCalculator{
		totalVotingPower: pc.totalVotingPower,
		lock:             &sync.RWMutex{},
		snapshot:         snapshot,
		round:            0,
		logger:           pc.logger,
	}

	return proposerCalc, snapshot
}

func (pc *proposerCalculator) incrementProposerPriorityNTimes(times uint64) error {
	if len(pc.snapshot.Validators) == 0 {
		return fmt.Errorf("validator set cannot be nul or empty")
	}

	if times <= 0 {
		return fmt.Errorf("cannot call IncrementProposerPriority with non-positive times")
	}

	if err := pc.rescalePriorities(); err != nil {
		return fmt.Errorf("cannot rescale priorities: %w", err)
	}

	if err := pc.shiftByAvgProposerPriority(); err != nil {
		return fmt.Errorf("cannot shift avg priorities: %w", err)
	}

	var (
		proposer *ProposerCalculatorValidator
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

func (pc *proposerCalculator) updateValidators(newValidatorSet AccountSet) {
	if newValidatorSet.Len() == 0 {
		return
	}

	oldProposerCalcValidators := pc.snapshot.Validators

	newValidatorsCalcProposer := make([]*ProposerCalculatorValidator, len(newValidatorSet))
	addressOldValidatorMap := make(map[types.Address]*ProposerCalculatorValidator, len(oldProposerCalcValidators))

	for _, x := range oldProposerCalcValidators {
		addressOldValidatorMap[x.Metadata.Address] = x
	}

	// create new validators snapshot
	for i, x := range newValidatorSet {
		priority := int64(0)

		// TODO: change priority if validator existed previous
		// if val, exists := addressOldValidatorMap[x.Address]; exists {
		// 	priority = val.ProposerPriority + int64(val.Metadata.VotingPower) - int64(x.VotingPower)
		// }

		newValidatorsCalcProposer[i] = &ProposerCalculatorValidator{
			Metadata:         x,
			ProposerPriority: priority,
		}
	}

	// TODO: centering

	pc.snapshot.Validators = newValidatorsCalcProposer
	pc.totalVotingPower = pc.snapshot.GetTotalVotingPower()
}

func (pc *proposerCalculator) incrementProposerPriority() (*ProposerCalculatorValidator, error) {
	for _, val := range pc.snapshot.Validators {
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

	for _, val := range pc.snapshot.Validators {
		val.ProposerPriority = safeSubClip(val.ProposerPriority, avgProposerPriority)
	}

	return nil
}

func (pc *proposerCalculator) getValWithMostPriority() (result *ProposerCalculatorValidator, err error) {
	if len(pc.snapshot.Validators) == 0 {
		return nil, fmt.Errorf("validators cannot be nil or empty")
	}

	for _, curr := range pc.snapshot.Validators {
		// pick curr as result if it has greater priority
		// or if it has same priority but "smaller" address
		if isBetterProposer(curr, result) {
			result = curr
		}
	}

	return result, nil
}

func (pc *proposerCalculator) computeAvgProposerPriority() (int64, error) {
	if len(pc.snapshot.Validators) == 0 {
		return 0, fmt.Errorf("validator set cannot be nul or empty")
	}

	n := int64(len(pc.snapshot.Validators))
	sum := big.NewInt(0)

	for _, val := range pc.snapshot.Validators {
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
	if len(pc.snapshot.Validators) == 0 {
		return fmt.Errorf("validator set cannot be nul or empty")
	}

	// Cap the difference between priorities to be proportional to 2*totalPower by
	// re-normalizing priorities, i.e., rescale all priorities by multiplying with:
	// 2*totalVotingPower/(maxPriority - minPriority)
	diffMax := priorityWindowSizeFactor * pc.totalVotingPower

	// Calculating ceil(diff/diffMax):
	// Re-normalization is performed by dividing by an integer for simplicity.
	// NOTE: This may make debugging priority issues easier as well.
	diff := computeMaxMinPriorityDiff(pc.snapshot.Validators)
	ratio := (diff + diffMax - 1) / diffMax

	if diff > diffMax {
		for _, val := range pc.snapshot.Validators {
			val.ProposerPriority /= ratio
		}
	}

	return nil
}

// Copy each validator into a new ValidatorSet.
func (pc proposerCalculator) Copy() *proposerCalculator {
	pc.lock.RLock()
	defer pc.lock.RUnlock()

	return &proposerCalculator{
		proposer:         pc.proposer,
		lock:             &sync.RWMutex{},
		totalVotingPower: pc.totalVotingPower,
		snapshot:         pc.snapshot.Copy(),
		logger:           pc.logger,
	}
}

// computeMaxMinPriorityDiff computes the difference between the max and min ProposerPriority of that set.
func computeMaxMinPriorityDiff(validators []*ProposerCalculatorValidator) int64 {
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
