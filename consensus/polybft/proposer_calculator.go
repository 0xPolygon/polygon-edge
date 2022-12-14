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

// Holds ValidatorMetadata together with priority
type PrioritizedValidator struct {
	Metadata         *ValidatorMetadata
	ProposerPriority int64
}

// ProposerSnapshot represents snapshot of one proposer calculation
type ProposerSnapshot struct {
	Height     uint64
	Round      uint64
	Proposer   *PrioritizedValidator
	Validators []*PrioritizedValidator
}

// NewProposerSnapshotFromState create ProposerSnapshot from state if possible or from genesis block
func NewProposerSnapshotFromState(config *runtimeConfig, logger hclog.Logger) (*ProposerSnapshot, error) {
	snapshot, err := config.State.getProposerSnapshot()
	if err != nil {
		return nil, err
	}

	if snapshot == nil {
		// pick validator set from genesis block if snapshot is not saved in db
		genesisValidatorsSet, err := config.polybftBackend.GetValidators(0, nil)
		if err != nil {
			return nil, err
		}

		snapshot = NewProposerSnapshot(1, genesisValidatorsSet)
	}

	return snapshot, nil
}

// NewProposerSnapshot creates ProposerSnapshot with height and validators with all priorities set to zero
func NewProposerSnapshot(height uint64, validators []*ValidatorMetadata) *ProposerSnapshot {
	validatorsSnap := make([]*PrioritizedValidator, len(validators))

	for i, x := range validators {
		validatorsSnap[i] = &PrioritizedValidator{Metadata: x, ProposerPriority: int64(0)}
	}

	return &ProposerSnapshot{
		Round:      0,
		Proposer:   nil,
		Height:     height,
		Validators: validatorsSnap,
	}
}

// Gets total voting power from all the validators
func (pcs ProposerSnapshot) GetTotalVotingPower() int64 {
	totalVotingPower := int64(0)

	for _, v := range pcs.Validators {
		totalVotingPower = safeAddClip(totalVotingPower, int64(v.Metadata.VotingPower))
	}

	return totalVotingPower
}

// Returns copy of current ProposerSnapshot object
func (pcs *ProposerSnapshot) Copy() *ProposerSnapshot {
	var proposer *PrioritizedValidator

	valCopy := make([]*PrioritizedValidator, len(pcs.Validators))

	for i, val := range pcs.Validators {
		valCopy[i] = &PrioritizedValidator{Metadata: val.Metadata.Copy(), ProposerPriority: val.ProposerPriority}

		if pcs.Proposer != nil && pcs.Proposer.Metadata.Address == val.Metadata.Address {
			proposer = valCopy[i]
		}
	}

	return &ProposerSnapshot{
		Validators: valCopy,
		Height:     pcs.Height,
		Round:      pcs.Round,
		Proposer:   proposer,
	}
}

// ProposerCalculator interface - proposer calculator algorithm should implement this interface
type ProposerCalculator interface {
	// CalcProposer calculates next proposer
	CalcProposer(snapshot *ProposerSnapshot, round, height uint64) (types.Address, error)

	// Update updates ProposerSnapshot to block with number `blockNumber`
	Update(snapshot *ProposerSnapshot, blockNumber uint64, config *runtimeConfig) error

	// GetLatestProposer returns latest calculated proposer if any
	GetLatestProposer(snapshot *ProposerSnapshot, round, height uint64) (types.Address, bool)
}

type proposerCalculator struct {
	// logger instance
	logger hclog.Logger
}

// NewProposerCalculator creates a new proposer calculator object
func NewProposerCalculator(logger hclog.Logger) *proposerCalculator {
	return &proposerCalculator{
		logger: logger.Named("proposer_calculator"),
	}
}

// GetLatestProposer returns latest calculated proposer if any
func (pc *proposerCalculator) GetLatestProposer(
	snapshot *ProposerSnapshot, round, height uint64) (types.Address, bool) {
	// round must be same as saved one and proposer must exist
	if snapshot.Proposer == nil || snapshot.Round != round || snapshot.Height != height {
		pc.logger.Info("Get latest proposer not found", "height", height, "round", round,
			"pc height", snapshot.Height, "pc round", snapshot.Round)

		return types.ZeroAddress, false
	}

	pc.logger.Info("Get latest proposer",
		"height", height, "round", round, "address", snapshot.Proposer.Metadata.Address)

	return snapshot.Proposer.Metadata.Address, true
}

// CalcProposer calculates next proposer
func (pc *proposerCalculator) CalcProposer(snapshot *ProposerSnapshot, round, height uint64) (types.Address, error) {
	if height != snapshot.Height {
		return types.ZeroAddress, fmt.Errorf("invalid height - expected %d, got %d", snapshot.Height, height)
	}

	// optimization -> return current proposer if already calculated for this round
	if snapshot.Round == round && snapshot.Proposer != nil {
		return snapshot.Proposer.Metadata.Address, nil
	}

	// do not change priorities on original snapshot while executing CalcProposer
	// if round = 0 then we need one iteration
	proposer, err := pc.incrementProposerPriorityNTimes(snapshot.Copy(), round+1)
	if err != nil {
		return types.ZeroAddress, err
	}

	snapshot.Proposer = proposer
	snapshot.Round = round

	pc.logger.Info("New proposer calculated", "height", height, "round", round, "address", proposer.Metadata.Address)

	return proposer.Metadata.Address, nil
}

// Update updates ProposerSnapshot to block with number `blockNumber`
func (pc *proposerCalculator) Update(snapshot *ProposerSnapshot, blockNumber uint64, config *runtimeConfig) error {
	if snapshot.Height != blockNumber {
		return fmt.Errorf("proposer calculator update wrong block=%d, height = %d",
			blockNumber, snapshot.Height)
	}

	currentHeader, found := config.blockchain.GetHeaderByNumber(blockNumber)
	if !found {
		return fmt.Errorf("cannot get header by number: %d", blockNumber)
	}

	extra, err := GetIbftExtra(currentHeader.ExtraData)
	if err != nil {
		return fmt.Errorf("cannot get ibft extra for block %d: %w", blockNumber, err)
	}

	var newValidatorSet AccountSet = nil

	if !extra.Validators.IsEmpty() {
		// TODO: optimize with parents
		newValidatorSet, err = config.polybftBackend.GetValidators(blockNumber, nil)
		if err != nil {
			return fmt.Errorf("cannot get ibft extra for block %d: %w", blockNumber, err)
		}
	}

	// if round = 0 then we need one iteration
	_, err = pc.incrementProposerPriorityNTimes(snapshot, extra.Checkpoint.BlockRound+1)
	if err != nil {
		return fmt.Errorf("failed to update calculator for block %d: %w", blockNumber, err)
	}

	// update to new validator set and center if needed
	pc.updateValidators(snapshot, newValidatorSet)

	snapshot.Height = blockNumber + 1
	snapshot.Round = 0
	snapshot.Proposer = nil

	pc.logger.Info("proposer calculator update has been finished",
		"height", blockNumber+1, "len", len(snapshot.Validators))

	return nil
}

func (pc *proposerCalculator) incrementProposerPriorityNTimes(
	snapshot *ProposerSnapshot, times uint64) (*PrioritizedValidator, error) {
	if len(snapshot.Validators) == 0 {
		return nil, fmt.Errorf("validator set cannot be nul or empty")
	}

	if times <= 0 {
		return nil, fmt.Errorf("cannot call IncrementProposerPriority with non-positive times")
	}

	var (
		proposer *PrioritizedValidator
		err      error
		tvp      = snapshot.GetTotalVotingPower()
	)

	if err := pc.updateWithChangeSet(snapshot, tvp); err != nil {
		return nil, err
	}

	for i := uint64(0); i < times; i++ {
		if proposer, err = pc.incrementProposerPriority(snapshot, tvp); err != nil {
			return nil, fmt.Errorf("cannot increment proposer priority: %w", err)
		}
	}

	snapshot.Proposer = proposer
	snapshot.Round = times - 1

	return proposer, nil
}

func (pc *proposerCalculator) updateValidators(snapshot *ProposerSnapshot, newValidatorSet AccountSet) {
	if newValidatorSet.Len() == 0 {
		return
	}

	oldProposerCalcValidators := snapshot.Validators

	newValidatorsCalcProposer := make([]*PrioritizedValidator, len(newValidatorSet))
	addressOldValidatorMap := make(map[types.Address]*PrioritizedValidator, len(oldProposerCalcValidators))

	for _, x := range oldProposerCalcValidators {
		addressOldValidatorMap[x.Metadata.Address] = x
	}

	// create new validators snapshot
	for i, x := range newValidatorSet {
		priority := int64(0)

		// TODO: change priority if validator existed previous
		if val, exists := addressOldValidatorMap[x.Address]; exists {
			priority = val.ProposerPriority // + int64(val.Metadata.VotingPower) - int64(x.VotingPower)
		}

		newValidatorsCalcProposer[i] = &PrioritizedValidator{
			Metadata:         x,
			ProposerPriority: priority,
		}
	}

	// TODO: centering

	snapshot.Validators = newValidatorsCalcProposer
}

func (pc *proposerCalculator) incrementProposerPriority(
	snapshot *ProposerSnapshot, totalVotingPower int64) (*PrioritizedValidator, error) {
	for _, val := range snapshot.Validators {
		// Check for overflow for sum.
		newPrio := safeAddClip(val.ProposerPriority, int64(val.Metadata.VotingPower))
		val.ProposerPriority = newPrio
	}
	// Decrement the validator with most ProposerPriority.
	mostest, err := pc.getValWithMostPriority(snapshot)
	if err != nil {
		return nil, fmt.Errorf("cannot get validator with most priority: %w", err)
	}

	mostest.ProposerPriority = safeSubClip(mostest.ProposerPriority, totalVotingPower)

	return mostest, nil
}

func (pc *proposerCalculator) updateWithChangeSet(snapshot *ProposerSnapshot, totalVotingPower int64) error {
	if err := pc.rescalePriorities(snapshot, totalVotingPower); err != nil {
		return fmt.Errorf("cannot rescale priorities: %w", err)
	}

	if err := pc.shiftByAvgProposerPriority(snapshot); err != nil {
		return fmt.Errorf("cannot shift proposer priorities: %w", err)
	}

	return nil
}

func (pc *proposerCalculator) shiftByAvgProposerPriority(snapshot *ProposerSnapshot) error {
	avgProposerPriority, err := pc.computeAvgProposerPriority(snapshot)
	if err != nil {
		return fmt.Errorf("cannot compute proposer priority: %w", err)
	}

	for _, val := range snapshot.Validators {
		val.ProposerPriority = safeSubClip(val.ProposerPriority, avgProposerPriority)
	}

	return nil
}

func (pc *proposerCalculator) getValWithMostPriority(
	snapshot *ProposerSnapshot) (result *PrioritizedValidator, err error) {
	if len(snapshot.Validators) == 0 {
		return nil, fmt.Errorf("validators cannot be nil or empty")
	}

	for _, curr := range snapshot.Validators {
		// pick curr as result if it has greater priority
		// or if it has same priority but "smaller" address
		if isBetterProposer(curr, result) {
			result = curr
		}
	}

	return result, nil
}

func (pc *proposerCalculator) computeAvgProposerPriority(snapshot *ProposerSnapshot) (int64, error) {
	if len(snapshot.Validators) == 0 {
		return 0, fmt.Errorf("validator set cannot be nul or empty")
	}

	n := int64(len(snapshot.Validators))
	sum := big.NewInt(0)

	for _, val := range snapshot.Validators {
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
func (pc *proposerCalculator) rescalePriorities(snapshot *ProposerSnapshot, totalVotingPower int64) error {
	if len(snapshot.Validators) == 0 {
		return fmt.Errorf("validator set cannot be nul or empty")
	}

	// Cap the difference between priorities to be proportional to 2*totalPower by
	// re-normalizing priorities, i.e., rescale all priorities by multiplying with:
	// 2*totalVotingPower/(maxPriority - minPriority)
	diffMax := priorityWindowSizeFactor * totalVotingPower

	// Calculating ceil(diff/diffMax):
	// Re-normalization is performed by dividing by an integer for simplicity.
	// NOTE: This may make debugging priority issues easier as well.
	diff := computeMaxMinPriorityDiff(snapshot.Validators)
	ratio := (diff + diffMax - 1) / diffMax

	if diff > diffMax {
		for _, val := range snapshot.Validators {
			val.ProposerPriority /= ratio
		}
	}

	return nil
}

// computeMaxMinPriorityDiff computes the difference between the max and min ProposerPriority of that set.
func computeMaxMinPriorityDiff(validators []*PrioritizedValidator) int64 {
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

func isBetterProposer(a, b *PrioritizedValidator) bool {
	if b == nil || a.ProposerPriority > b.ProposerPriority {
		return true
	} else if a.ProposerPriority == b.ProposerPriority {
		return bytes.Compare(a.Metadata.Address.Bytes(), b.Metadata.Address.Bytes()) <= 0
	} else {
		return false
	}
}
